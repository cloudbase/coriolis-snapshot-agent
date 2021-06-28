# Coriolis snapshot agent

Coriolis snapshot agent leverages the [blk-snap](https://patchwork.kernel.org/project/linux-block/cover/1603271049-20681-1-git-send-email-sergei.shtepa@veeam.com/) kernel module to create copy-on-write snapshots of block devices, on a GNU/Linux system, which are then consumed by [Coriolis](https://github.com/cloudbase/coriolis). The process by which snapshots are created is similar to how [VSS](https://docs.microsoft.com/en-us/windows-server/storage/file-server/volume-shadow-copy-service) works on Windows.


## The blk-snap linux kernel module

The blk-snap kernel module offerts block level copy-on-write snapshot functionality. To achieve this, it leverages block filters to intercept BIO requests to a block device. A detailed description of how it works can be found on the [linux kernel mailing list](https://lkml.org/lkml/2020/10/21/122).

## Basic concepts

Before we dive into how the agent works, it's important to undertand the underlying functionality it leverages. This way we get a better sense of what is possible, how to pepare our system to perform a backup, as well as troubleshoot any potential issues that may come up. There are several layers to the kernel module, that we need to get familiar with before we start using it.

### CBT

CBT stands for Change Block Tracking. The concept of CBT has been around for a while, and is widely used in products such as VMware ESXi to create incremental backups of virtual machine disks. It works by creating a bitmap, where each element of the array, represents one ```block```. Whenever something in that block changes, the entire block is marked as "dirty", and the value of the byte representing that block is set to the snapshot number in which it was changed. Let's look at an example to get a better idea of what this means.


Say you add a new device under tracking. When you do that, the kernel module inspects the block volume, and determins the number of sectors it has. Sectors in the linux kernel are always 512 bytes, regardless of the physical sector size your storage volume reports. Taking into account that the block size is 16 KB, it creates a new bitmap in memory with enough elements to represent each block. When the bitmap is created, all elements are set to 0, which looks something like this:

```go
[0 0 0 0 0 0 0 0 0 0]
```

Now let's pretend that we've just taken a snapshot. That snapshot receives a number. It's the first snapshot, so the number will be ```1```. At this point, if we don't change anything on disk, the bitmap stays unchanged. If however, we change something, the kernel module will record that change and set the element representing that block, to the number of the current snapshot. So, after changing something within one block, the bitmap may look something like this:

```go
[0 0 0 0 1 0 0 0 0 0]
```

If we record the snapshot number, in a subsequent backup operation, we can look at the bitmap and only copy over the blocks that have changed from the previous backup. This allows us to do efficient, incremental backups of physical disks.

There are a few things to keep in mind though:

  * There is currently no way to persist CBT data between reboots. Everything is kept in RAM, so if the system reboots, you lose your bitmaps and tracking info and you have to do a full sync.
  * Each element of the array is one byte. That means you can only keep track of 255 consecutive snapshots, after which the bitmap resets.

To know when a CBT bitmap has been reset, the kernel module adds a ```uuid4``` unique identifier to the CBT bitmap itself, called a **generation ID**. If the generation ID you recorded after a previous backup is different from the current gneration ID of the block volume, you know you have to do a full sync.  

### Tracking

Within the kernel module, tracking is the process by which an individual block volume is initialized with a CBT bitmap, and a block level filter is installed to intercept BIO requests. You can choose to track an entire block volume, a single partition of that block volume or a device mapper that spans across multiple disks.

Coriolis treats all machines it migrates as black boxes. So this agent will always track raw block volumes, regardless of what they contain. We do not care about device mappers or filesystems residing on those volumes.

### Snap store

As mentioned, the kernel module enables block level copy-on-write snapshots of physical disks on a running linux system. But any copy-on-write system needs a place to copy any changed blocks that need to remain private to a particular snapshot. That is where snap stores come in. A snap store is a container in which the CoW data can be stored. The kernel module gives us 3 options here:

  * Use a chunk of memory as a destination. This is great for testing, not so great for systems that do not have huge amounts of unused RAM.
  * Allocate a chunk of the disk we are currently snapshotting as a snap store. To safely do this, without causing a deadlock, the kernel module allows us to allocate a number of ranges that will be ignored by the kernel module when they change.
  * Use a separate physical disk.

In this initial release of the agent, we require that you have a separate block device as a snap store destination.

### Why a separate disk?!

Coriolis treats the systems it migrates as black boxes. As a result, we want to be able to copy over raw disks in their entirety from source to destination. That means we want **all** the information on those disks to be copied to our destination, regardless of whether they are plain disks with a dos or GPT partition table, if they are part of a software RAID or LVM2 group, etc. We want to be able to have a 1:1 copy of the entire disk array.

To safely allocate disk ranges to be used as CoW destinations, we need to do that through the filesystem that sits on top of the block volumes themselves. We can't simply choose arbitrary ranges of disks, because we risk overwriting critical information on the disk, such as a superblock, LVM metadata, or even a partition table. This means that the safest way to reserve chunks of disk is to go through the filesystem and create large files. The simplest way to do that is by using ```fallocate``` on filesystems that support this operation. Another way is to use ```dd```, or any tool that actually allocates the disk. Note, sparse files won't do it. By their nature, a sparse file will not really take up any space, until you actually write something inside of it. Using ```fallocate``` on the other hand, will reserve the disk chunks you need. Here's an example:

Create a sparse file:

```shell
gabriel@rossak:/tmp$ truncate -s 2048M ./sparse_test
```

Check the space use by the sparse file:
```shell
gabriel@rossak:/tmp$ du -sh ./sparse_test 
0	./sparse_test
```

Check the extents used by the sparse file:

```shell
gabriel@rossak:/tmp$ sudo hdparm --fibmap ./sparse_test 

./sparse_test:
 filesystem blocksize 4096, begins at LBA 532480; assuming 512 byte sectors.
 byte_offset  begin_LBA    end_LBA    sectors
 ```

As you can see, there is nothing we can use.

Now let's do the same using fallocate.

Allocate space using fallocate:
```shell
gabriel@rossak:/tmp$ fallocate -l 2048M ./fallocate_test
```

Check disk space usage:

```shell
gabriel@rossak:/tmp$ du -sh ./fallocate_test 
2.1G	./fallocate_test
```

Get the extents allocated to the file

``` shell
./fallocate_test:
 filesystem blocksize 4096, begins at LBA 532480; assuming 512 byte sectors.
 byte_offset  begin_LBA    end_LBA    sectors
           0   52699136   55058431    2359296
  1207959552   55320576   55353343      32768
  1224736768   55386112   57188351    1802240

```

In this instance, we have 3 ranges of continuous bytes we can feed into the kernel module to be used as CoW destinations. But here is the catch with this approach, when taking into account the fact that we have a requirement to copy raw disks in their entirety. If the filesystem is on a device mapper, the sector ranges printed above, will probably not match those of the underlying devices we are tracking. Device mapper by it's nature, re-maps sectors from multiple individual block devices, into a different device, potentially larger device (LVM2 for example). As a result if the filesystem resides on a logical volume, the ranges that are reported by the operating system, are those of the device mapper, not those of the underlying disk.

Since Coriolis doesn't care about what device mapper volumes you have, we need to unmap those sectors and get the underlying physical sectors they actually point to, because we instruct the kernel module to track the entire, individual disks, not just a partition of those disks, or a device mapper. For example, say we have a LVM2 volume group, spanning 2 disks. Say you want to allocate ranges starting from sector 1000 to sector 1200. From the perspective of the logical volume, those ranges are continuous, but from the perspective of the underlying disks the device mapper maps to, that may mean sectors 800-900 on ```/dev/sda``` and sectors 0-100 on ```/dev/sdb```.

This functionality is not yet part of this agent, but will most likely be added in a subsequent release. Until then, you will need to add a new physical disk or an iSCSI taget/rbd device.


## Kernel module interfaces

The kernel module exposes two interfaces with userspace:

  * Character device operations
  * ioctl

### The character device

The kernel module exposes a character device called ```/dev/blk-snap```. A new 2-way communication pipe gets created whenever you issue an ```open()``` request on this file. Through this interface, you can choose to use the character device commands, or execute ```ioctl``` requests. The character device interface only offers a small subset of what you can do through ioctl, namely snapstore creation and expansion. We leverage this interface in the agent, because we can set a threshold at which the kernel module lets us know that we need to add more space to the snap store to prevent an overflow event. The interface exposes the following commands:

  * Initiate. Through this command we create a new snap store and create a pipe through which we'll receive updates about that snap store, as well as send new commands.
  * Next portion. This command allows us to add new disk ranges to the snap store
  * next portion multi dev. Snap stores can be comprised of one block volume, or multiple block volumes.

The following notifications are sent by the kernel module through the 2-way pipe that was created:

  * Half fill. This event indicates that the snap store is almost full. When creating a snap store though the character device, we have the option of setting a minimum threshold. The threashold is expressed in bytes of free space, that when reached, we should be notified. Say you want to be notified when the snap store only has 1 GB of disl space available, so you can add another 10 GB. When that threshold is reached, this event is triggered and a message is sent through the character device.
  * Overflow. This event is triggered when the snap store ran out of disk space to place any new CoW extents. When this happens, your snapshot will become corrupt and you will most likely have to recreate it.
  * Teminate. This event is triggered when the snap store was deleted. We use this event to know when we need to clean up any allocated files.

## Snapstores and watchers

Whenever a new snapshot is created, the agent will create a snap store. The health of that snap store is monitored by a watcher the agent spawns. If the snap store starts to run out of disk space, it is the job of the watcher to add more space, until the backup process is complete or until the snap store device runs out of space. The watcher also cleans up any allocated ranges, after the snap store is deleted.

### What happens if I restart the agent?

It's safe to restart the agent without cleaning up any snapshots or snap stores beforehand. The agent persists all info about resources it creates, in a local database. If restarted, it will reattach itself to the character device and register the needed watchers.

### What kind of database does the agent use?

The agent uses a [bbolt](https://github.com/etcd-io/bbolt), key-value part database. The database itself is hosted on a ```tmpfs``` filesystem (/var/run). The reason we don't want to persist the database between reboots, is because there is currently no way to persist the CBT info between reboots. So if we reboot the system, we need to start from scratch anyway. It's easier to start with a clean database, than to cleanup all the old entries from a DB that persists between reboots.

## Instalation

### Kernel module instalation

Clone the module:

```bash
git clone https://github.com/veeam/veeamsnap
```

Install the module:

```bash
cd veeamsnap/source
make
make install
```

If you're on a debian based system:

```bash
make dkms-deb-pkg
dpkg -i ../veeamsnap_5.0.0.0_all.deb
```

Add the module to ```/etc/modules```. This will load it at boot:

```bash
cat /etc/modules
veeamsnap
```

Create persistend udev rules to grant the right permissions on the character device:

```bash
# /etc/udev/rules.d/99-veeamsnap.rules
KERNEL=="veeamsnap", OWNER="root", GROUP="disk"
```

### Coriolis snapshot agent instalation

Build the binary. You will need to have docker installed:

```bash
make
```

After the command returns, you'll have a statically built binary in your current folder. simply copy the binary anywhere in your ```$PATH```:

```bash
cp coriolis-snapshot-agent /usr/local/bin
```

Add a user:

```bash
useradd --system --home-dir=/nonexisting --groups disk --no-create-home --shell /bin/false coriolis
```

Create a service definition:

```bash
cp contrib/coriolis-snapshot-agent.service.sample /etc/systemd/system/coriolis-snapshot-agent.service
systemctl daemon-reload
```

Create the config folder:

```bash
mkdir /etc/coriolis-snapshot-agent
```

Copy and edit the sample config

```bash
cp contrib/config.toml.sample /etc/coriolis-snapshot-agent/config.toml
```

The agent uses client x509 certificates for authentication. To gain access to the API, you must generate proper server and client certificates. The server certificate and the CA certificate must be correctly configured in the above mentioned config file. Here is a quick and dirty way to generate the certificates for testing:

```bash
wget https://gist.githubusercontent.com/gabriel-samfira/61663ec3c07652d4deeeccfdec319d64/raw/ba1a37dedeb224516b0c44fb4c171ac4c8ed1f10/gen_certs.go
go build ./gen_certs.go

sudo mkdir -p /etc/coriolis-snapshot-agent/certs
sudo ./gen_certs -output-dir /etc/coriolis-snapshot-agent/certs -certificate-hosts localhost,127.0.0.1
```

Change ownership of folder:

```bash
chown coriolis:disk -R /etc/coriolis-snapshot-agent
```

Enable and start the service:

```bash
systemctl daemon-reload
systemctl enable coriolis-snapshot-agent
systemctl start coriolis-snapshot-agent
```

### The agent config

The comments are sufficiently detailed. A copy of this config sample is present in the contrib folder.

```toml
# Path to coriolis snapshot agent log file
log_file = "/tmp/coriolis-snapshot-agent.log"

# snapstore_destinations is an array of paths on disk where the snap
# store watchers will allocate disk space for the snap stores. The device
# on which these folders reside, will be excluded from the list of
# snapshot-able disks. If this path is on a device mapper, all disks
# that make up that device mapper, will be excluded. Paths set here, should
# be on a separate block volume (physical, iSCSI, rbd, etc). 
snapstore_destinations = ["/mnt/snapstores/snapstore_files"]
auto_init_physical_disks = true

# Snapstore mappings are a quick way to pre-configure snap store mappings.
# When creating a snapshot, the agent will look for a mapping of where it
# could define a new snap store to hold the CoW chunks for a disk. If no
# mappings are specified here, before you can take a snapshot of a disk,
# you must create a snapstore mapping through the API. Considering that
# disks do not really change, the best way to go about this is to define the
# mappings in the config.
[[snapstore_mapping]]
# device is the device name for which we need to create a mapping for.
device = "vda"
# location must be one of the locations configured in the snapstore_destinations
# option above.
location = "/mnt/snapstores/snapstore_files"

[[snapstore_mapping]]
device = "vdc"
location = "/mnt/snapstores/snapstore_files" 

[api]
# IP address to bind to
bind = "0.0.0.0"
# Port to listen on
port = 9999
	[api.tls]
	# x509 settings for this daemon. The agent will validate client
	# certificates before answering to API requests.
	certificate = "/etc/coriolis-snapshot-agent/ssl/srv-pub.pem"
	key = "/etc/coriolis-snapshot-agent/ssl/srv-key.pem"
	ca_certificate = "/etc/coriolis-snapshot-agent/ssl/ca-pub.pem"

```

## Agent API

