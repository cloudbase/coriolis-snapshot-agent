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

To know when a CBT bitmap has been reset, the kernel module adds a ```uuid4``` unique identifier to the CBT bitmap itself, called a **generation ID**. If the generation ID you recorded after a previous backup is different from the current generation ID of the block volume, you know you have to do a full sync.

### Tracking

Within the kernel module, tracking is the process by which an individual block volume is initialized with a CBT bitmap, and a block level filter is installed to intercept BIO requests. You can choose to track an entire block volume, a single partition of that block volume or a device mapper that spans across multiple disks.

Coriolis treats all machines it migrates as black boxes. So this agent will always track raw block volumes, regardless of what they contain. We do not care about device mappers or filesystems residing on those volumes.

WARNING! The agent does not support tracking or transferring swap or virtual disks. The aim of this agent is covering migration of physical servers to virtual space. There is no point in tracking changes of such disks, therefore the agent filters them out. The lack of swap disks should not interfere with the server booting on a destination platform, but its performance should still be checked, and, if needed, eventually raise the destination machine's memory.

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
0    ./sparse_test
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
2.1G    ./fallocate_test
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

In this instance, we have 3 ranges of continuous bytes we can feed into the kernel module to be used as CoW destinations. But here is the catch with this approach, when taking into account the fact that we have a requirement to copy raw disks in their entirety. If the filesystem is on a device mapper, the sector ranges printed above, will probably not match those of the underlying devices we are tracking. Device mapper by its nature, re-maps sectors from multiple individual block devices, into a different device, potentially larger device (LVM2 for example). As a result if the filesystem resides on a logical volume, the ranges that are reported by the operating system, are those of the device mapper, not those of the underlying disk.

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

  * Half fill. This event indicates that the snap store is almost full. When creating a snap store though the character device, we have the option of setting a minimum threshold. The threashold is expressed in bytes of free space, that when reached, we should be notified. Say you want to be notified when the snap store only has 1 GB of disk space available, so you can add another 10 GB. When that threshold is reached, this event is triggered and a message is sent through the character device.
  * Overflow. This event is triggered when the snap store ran out of disk space to place any new CoW extents. When this happens, your snapshot will become corrupt and you will most likely have to recreate it.
  * Teminate. This event is triggered when the snap store was deleted. We use this event to know when we need to clean up any allocated files.

## Snapstores and watchers

Whenever a new snapshot is created, the agent will create a snap store. The health of that snap store is monitored by a watcher the agent spawns. If the snap store starts to run out of disk space, it is the job of the watcher to add more space, until the backup process is complete or until the snap store device runs out of space. The watcher also cleans up any allocated ranges, after the snap store is deleted.

### What happens if I restart the agent?

It's safe to restart the agent without cleaning up any snapshots or snap stores beforehand. The agent persists all info about resources it creates in a local database. If restarted, it will reattach itself to the character device and register the needed watchers.

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

Create the snapshot files destination. The destination needs to be mounted on a separate disk, that will not be snapshotted.

```bash
mkdir -p /mnt/snapstores/snapstore_files
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
# on which these folders reside will be excluded from the list of
# snapshot-able disks. If this path is on a device mapper, all disks
# that make up that device mapper will be excluded. Paths set here should
# be on a separate block volume (physical, iSCSI, rbd, etc).
snapstore_destinations = ["/mnt/snapstores/snapstore_files"]

# auto_init_physical_disks, if true will automatically add all physical
# disks that are not set as a snap store destination under tracking.
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

# Snap store file size is the size in bytes of the chunks of disk space that will
# be added to a snap store in the event that a snap store reaches their
# "empty limit". The empty limit is a threshold set on every created snap store
# that is equal to the size of this setting, where the agent gets notified that
# it needs to add more disk space. The disk space it adds is in increments of
# bytes, dictated by this option. The default value is 2 GB.
# So for example, if you set this option to 2 GB, if the disk space available
# to a snap store drops bellow 2 GB, an event is triggered that prompts the agent
# to add another 2 GB chunk of space to a snap store.
snap_store_file_size = 2147483648

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

### List disks

```
GET /api/v1/disks/
```

| Name | Type | Optional | Description |
| ---- | ---- | -------- | ----------- |
| includeVirtual | bool | true | When true, the API will return all block devices on the system, except the ones used for snap stores |

Example usage:


```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/disks/|jq
[
  {
    "id": "vda",
    "path": "/dev/vda",
    "partition_table_type": "gpt",
    "partition_table_uuid": "8df09209-6e27-4f0c-a155-ad327bb8f89b",
    "name": "vda",
    "size": 26843545600,
    "logical_sector_size": 512,
    "physical_sector_size": 512,
    "partitions": [
      {
        "name": "vda1",
        "path": "/dev/vda1",
        "sectors": 2048,
        "partition_uuid": "52296511-3cf6-497c-a96c-2c4b5dd6e540",
        "partition_type": "21686148-6449-6e6f-744e-656564454649",
        "start_sector": 2048,
        "end_sector": 4095,
        "alignment_offset": 0,
        "device_major": 252,
        "device_minor": 1
      },
      {
        "name": "vda2",
        "path": "/dev/vda2",
        "sectors": 2097152,
        "filesystem_uuid": "0f77716b-36b3-4fec-bc41-6855b9e6fbd3",
        "partition_uuid": "b662da27-fe4d-41a3-99d4-d9173349378d",
        "partition_type": "0fc63daf-8483-4772-8e79-3d69d8477de4",
        "filesystem_type": "ext4",
        "start_sector": 4096,
        "end_sector": 2101247,
        "alignment_offset": 0,
        "device_major": 252,
        "device_minor": 2
      },
      {
        "name": "vda3",
        "path": "/dev/vda3",
        "sectors": 50325504,
        "filesystem_uuid": "zKQWwb-zJei-IhET-NX3W-lfNC-lF7U-q5Da5D",
        "partition_uuid": "fad5320e-cf4d-4117-96a9-4b219c9f9065",
        "partition_type": "0fc63daf-8483-4772-8e79-3d69d8477de4",
        "filesystem_type": "LVM2_member",
        "start_sector": 2101248,
        "end_sector": 52426751,
        "alignment_offset": 0,
        "device_major": 252,
        "device_minor": 3
      }
    ],
    "filesystem_type": "",
    "alignment_offset": 0,
    "device_major": 252,
    "is_virtual": false
  },
  {
    "id": "vdc",
    "path": "/dev/vdc",
    "partition_table_type": "dos",
    "partition_table_uuid": "73ac85a2",
    "name": "vdc",
    "size": 21474836480,
    "logical_sector_size": 512,
    "physical_sector_size": 512,
    "partitions": [
      {
        "name": "vdc1",
        "path": "/dev/vdc1",
        "sectors": 41940992,
        "filesystem_uuid": "2bc1143a-f7df-4dd3-b39e-6113b4f4c479",
        "partition_uuid": "73ac85a2-01",
        "partition_type": "0x83",
        "filesystem_type": "ext4",
        "start_sector": 2048,
        "end_sector": 41943039,
        "alignment_offset": 0,
        "device_major": 252,
        "device_minor": 33
      }
    ],
    "filesystem_type": "",
    "alignment_offset": 0,
    "device_major": 252,
    "device_minor": 32,
    "is_virtual": false
  }
]
```

### Get single disk

```bash
GET /api/v1/disks/{diskTrackingID}/
```

Example usage:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/disks/vdc/|jq
{
  "id": "vdc",
  "path": "/dev/vdc",
  "partition_table_type": "dos",
  "partition_table_uuid": "73ac85a2",
  "name": "vdc",
  "size": 21474836480,
  "logical_sector_size": 512,
  "physical_sector_size": 512,
  "partitions": [
    {
      "name": "vdc1",
      "path": "/dev/vdc1",
      "sectors": 41940992,
      "filesystem_uuid": "2bc1143a-f7df-4dd3-b39e-6113b4f4c479",
      "partition_uuid": "73ac85a2-01",
      "partition_type": "0x83",
      "filesystem_type": "ext4",
      "start_sector": 2048,
      "end_sector": 41943039,
      "alignment_offset": 0,
      "device_major": 252,
      "device_minor": 33
    }
  ],
  "filesystem_type": "",
  "alignment_offset": 0,
  "device_major": 252,
  "device_minor": 32,
  "is_virtual": false
}
```

### View snap store locations

The response from this call should mirror the values set in your config under the ```snapstore_destinations``` option.

```bash
GET /api/v1/snapstorelocations/
```

Example usage:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapstorelocations/|jq
[
  {
    "available_capacity": 39791616000,
    "allocated_capacity": 0,
    "total_capacity": 42006183936,
    "path": "/mnt/snapstores/snapstore_files",
    "device_path": "/dev/vdb1",
    "major": 252,
    "minor": 17
  }
]
```

The unique identifier is the ```path``` field.


### View snap store mappings

If you configured ```snapstore_mapping``` sections in your config file, this should already be populated.

```bash
GET /api/v1/snapstoremappings/
```

Example usage:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapstoremappings/|jq
[
  {
    "id": "4df3cb1d-aded-4a08-8a01-aa1464bd6a65",
    "tracked_disk_id": "vda",
    "storage_location": "/mnt/snapstores/snapstore_files"
  },
  {
    "id": "eedef3c4-b716-410c-803f-4484a34e9290",
    "tracked_disk_id": "vdc",
    "storage_location": "/mnt/snapstores/snapstore_files"
  }
]
```

### Create a snap store mapping

If you have not configured any snap store mappings in your config, you can still add them via the API.

```bash
POST /api/v1/snapstoremappings/
```

Example usage:

```bash
curl -0 -X POST https://192.168.122.87:9999/api/v1/snapstoremappings/ \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
    -H "Content-type: application-json" \
    --data-binary @- << EOF
    {
        "snapstore_location_id": "/mnt/snapstores/snapstore_files",
        "tracked_disk_id": "vdc"
    }
EOF
```

### Create snapshot

Now that we have our snap store mappings set up, we can create a snapshot.

```bash
POST /api/v1/snapshots/
```

Example usage:

```bash
curl -s -X POST https://192.168.122.87:9999/api/v1/snapshots/ \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
    -H "Content-type: application-json" \
    --data-binary @- << EOF
    {
        "tracked_disk_ids": ["vda", "vdc"]
    }
EOF
```

This will create a **single** snapshot, encompasing **two** disks. Each disk will have its own snapshot volume, but both snapshot volumes will be identified by the same snapshot ID. Naturally, you can create snapshots of each individual disks if you so wish.

The operations that take place when creating a snapshot are as follows:

  * For each disk in the array, a new snap store is created in the location indicated by the snap store mapping.
  * The snapstore will get an initial disk space allocation of 20% of the size of the disk that is being snapshot.
  * A new snap store watcher is spawned internally, that will monitor the status of disk usage during the backup operation.
  * A snapshot is created and the details describing that snapshot are returned as part of the response.

  ### List snapshots

  ```bash
  GET /api/v1/snapshots/
  ```

Example usage:

```bash
curl -s -X GET --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapshots/|jq
[
  {
    "SnapshotID": "18446633009895023040",
    "VolumeSnapshots": [
      {
        "SnapshotNumber": 3,
        "GenerationID": "80564045-085a-4b1c-82d4-7410bce63d4a",
        "OriginalDevice": {
          "TrackingID": "vda",
          "DevicePath": "/dev/vda",
          "Major": 252,
          "Minor": 0
        },
        "SnapshotImage": {
          "DevicePath": "/dev/veeamimage0",
          "Major": 251,
          "Minor": 0
        }
      },
      {
        "SnapshotNumber": 2,
        "GenerationID": "3e5fb057-e312-4784-80ed-49b4b8117f79",
        "OriginalDevice": {
          "TrackingID": "vdc",
          "DevicePath": "/dev/vdc",
          "Major": 252,
          "Minor": 32
        },
        "SnapshotImage": {
          "DevicePath": "/dev/veeamimage1",
          "Major": 251,
          "Minor": 1
        }
      }
    ]
  }
]
```

### Delete snapshot

```bash
DELETE /api/v1/snapshots/{snapshotID}/
```

Example usage:

```bash
curl -s -X DELETE \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapshots/18446633009895023040/
```

### Get snapshot changes

This endpoint allows you to fetch a list of changes from a previous snapshot. If you do not have a previous snapshot, this endpoint will return one big range, encompasing the entire disk.

```bash
GET /api/v1/snapshots/{snapshotID}/changes/{trackedDiskID}/
```

| Name | Type | Optional | Description |
| ---- | ---- | -------- | ----------- |
| previousGenerationID | string | true | The generation ID of the previous snapshot. |
| previousNumber | int | true | The number of the previous snapshot. |

Get entire disk example:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapshots/18446633009963518464/changes/vda|jq
{
  "tracked_disk_id": "vda",
  "snapshot_id": "18446633009963518464",
  "cbt_block_size_bytes": 262144,
  "backup_type": "full",
  "ranges": [
    {
      "start_offset": 0,
      "length": 26843545600
    }
  ]
}
```

Get changes from previous snapshot:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  "https://192.168.122.87:9999/api/v1/snapshots/18446633009963518464/changes/vda/?previousGenerationID=80564045-085a-4b1c-82d4-7410bce63d4a&previousNumber=3"|jq
{
  "tracked_disk_id": "vda",
  "snapshot_id": "18446633009963518464",
  "cbt_block_size_bytes": 262144,
  "backup_type": "incremental",
  "ranges": [
    {
      "start_offset": 1076887552,
      "length": 262144
    },
    {
      "start_offset": 7519338496,
      "length": 262144
    },
    {
      "start_offset": 7519862784,
      "length": 262144
    },
    {
      "start_offset": 7563378688,
      "length": 262144
    },
    {
      "start_offset": 11967135744,
      "length": 1048576
    },
    {
      "start_offset": 16109273088,
      "length": 262144
    },
    {
      "start_offset": 16115564544,
      "length": 262144
    },
    {
      "start_offset": 16275734528,
      "length": 262144
    },
    {
      "start_offset": 17999593472,
      "length": 262144
    },
    {
      "start_offset": 18000642048,
      "length": 262144
    },
    {
      "start_offset": 18122539008,
      "length": 4194304
    },
    {
      "start_offset": 18259640320,
      "length": 262144
    },
    {
      "start_offset": 20404240384,
      "length": 262144
    },
    {
      "start_offset": 20405026816,
      "length": 262144
    },
    {
      "start_offset": 20407648256,
      "length": 786432
    },
    {
      "start_offset": 20591673344,
      "length": 262144
    },
    {
      "start_offset": 20702035968,
      "length": 524288
    },
    {
      "start_offset": 20703084544,
      "length": 262144
    },
    {
      "start_offset": 20703608832,
      "length": 524288
    },
    {
      "start_offset": 20704919552,
      "length": 786432
    }
  ]
}
```

### Download snapshot data

This endpoint allow you to download ranges of individual chunks of a particular snapshot.

```bash
GET /api/v1/snapshots/{snapshotID}/consume/{trackedDiskID}/
```

Example usage:

```bash
curl -s -X GET \
  -r 20704919552-20705705983 \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/snapshots/18446633009963518464/consume/vda/ > /tmp/chunk
```

You should have a chunk of data in ```/tmp/chunk```, representing the last range from the previous example.

```bash
$ ls -lh /tmp/chunk
-rw-rw-r-- 1 gabriel gabriel 768K Jun 28 14:38 /tmp/chunk
```

### Fetch system info

This endpoint returns information about the system. This includes:

  * Platform
  * OS name and version
  * CPU information
  * Block volume info
  * Memory info
  * Network interface information (HW address, name, IP addresses)

```bash
GET /api/v1/systeminfo/
```

Example usage:

```bash
curl -s -X GET \
  --cert /etc/coriolis-snapshot-agent/ssl/client-pub.pem \
  --key /etc/coriolis-snapshot-agent/ssl/client-key.pem \
  --cacert /etc/coriolis-snapshot-agent/ssl/ca-pub.pem \
  https://192.168.122.87:9999/api/v1/systeminfo|jq
{
  "memory": {
    "total": 4125925376,
    "available": 3045695488,
    "used": 796946432,
    "usedPercent": 19.315580369818107,
    "free": 1758420992,
    "active": 1409576960,
    "inactive": 567996416,
    "wired": 0,
    "laundry": 0,
    "buffers": 134098944,
    "cached": 1436459008,
    "writeBack": 0,
    "dirty": 0,
    "writeBackTmp": 0,
    "shared": 1368064,
    "slab": 243761152,
    "sreclaimable": 124612608,
    "sunreclaim": 119148544,
    "pageTables": 7319552,
    "swapCached": 0,
    "commitLimit": 6190153728,
    "committedAS": 2066169856,
    "highTotal": 0,
    "highFree": 0,
    "lowTotal": 0,
    "lowFree": 0,
    "swapTotal": 4127191040,
    "swapFree": 4127191040,
    "mapped": 210579456,
    "vmallocTotal": 35184372087808,
    "vmallocUsed": 20566016,
    "vmallocChunk": 0,
    "hugePagesTotal": 0,
    "hugePagesFree": 0,
    "hugePageSize": 2097152
  },
  "cpus": {
    "physical_cores": 4,
    "logical_cores": 8,
    "cpu_info": [
      {
        "cpu": 0,
        "vendorId": "GenuineIntel",
        "family": "6",
        "model": "94",
        "stepping": 3,
        "physicalId": "0",
        "coreId": "0",
        "cores": 1,
        "modelName": "Intel Core Processor (Skylake, IBRS)",
        "mhz": 2591.998,
        "cacheSize": 16384,
        "flags": [
          "vmx",
          ..... truncated ....
        ],
        "microcode": "0x1"
      },
      ...... truncated .............
  },
  "network_interfaces": [
    {
      "mac_address": "52:54:00:ab:b2:84",
      "ip_addresses": [
        "192.168.122.87/24",
        "fe80::5054:ff:feab:b284/64"
      ],
      "nic_name": "enp1s0"
    },
    {
      "mac_address": "02:42:74:8a:aa:48",
      "ip_addresses": [
        "172.17.0.1/16",
        "fe80::42:74ff:fe8a:aa48/64"
      ],
      "nic_name": "docker0"
    }
  ],
  "disks": [
    {
      "path": "/dev/vda",
      "partition_table_type": "gpt",
      "partition_table_uuid": "8df09209-6e27-4f0c-a155-ad327bb8f89b",
      "name": "vda",
      "size": 26843545600,
      "logical_sector_size": 512,
      "physical_sector_size": 512,
      "partitions": [
        {
          "name": "vda1",
          "path": "/dev/vda1",
          "sectors": 2048,
          "partition_uuid": "52296511-3cf6-497c-a96c-2c4b5dd6e540",
          "partition_type": "21686148-6449-6e6f-744e-656564454649",
          "start_sector": 2048,
          "end_sector": 4095,
          "alignment_offset": 0,
          "device_major": 252,
          "device_minor": 1
        },
        {
          "name": "vda2",
          "path": "/dev/vda2",
          "sectors": 2097152,
          "filesystem_uuid": "0f77716b-36b3-4fec-bc41-6855b9e6fbd3",
          "partition_uuid": "b662da27-fe4d-41a3-99d4-d9173349378d",
          "partition_type": "0fc63daf-8483-4772-8e79-3d69d8477de4",
          "filesystem_type": "ext4",
          "start_sector": 4096,
          "end_sector": 2101247,
          "alignment_offset": 0,
          "device_major": 252,
          "device_minor": 2
        },
        {
          "name": "vda3",
          "path": "/dev/vda3",
          "sectors": 50325504,
          "filesystem_uuid": "zKQWwb-zJei-IhET-NX3W-lfNC-lF7U-q5Da5D",
          "partition_uuid": "fad5320e-cf4d-4117-96a9-4b219c9f9065",
          "partition_type": "0fc63daf-8483-4772-8e79-3d69d8477de4",
          "filesystem_type": "LVM2_member",
          "start_sector": 2101248,
          "end_sector": 52426751,
          "alignment_offset": 0,
          "device_major": 252,
          "device_minor": 3
        }
      ],
      "filesystem_type": "",
      "alignment_offset": 0,
      "device_major": 252,
      "is_virtual": false
    },
    .... truncated ....
  ],
  "os_info": {
    "platform": "linux",
    "os_name": "ubuntu",
    "os_version": "20.04"
  },
  "hostname": "server01"
}

```
