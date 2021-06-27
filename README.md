# Coriolis snapshot agent

Coriolis snapshot agent leverages the [veeamsnap](https://github.com/veeam/veeamsnap) kernel module to create copy-on-write snapshots of block devices, on a GNU/Linux system. The process by which this happens is similar to how [VSS](https://docs.microsoft.com/en-us/windows-server/storage/file-server/volume-shadow-copy-service) works on Windows.


## Snapstores and watchers

Whenever a new snapshot is created, the agent will create a snap store. A snap store is a container for the copy-on-write pages to be hosted in. After taking a snapshot, whenever a block changes on a device that has a snapshot associated with it, that block is first copied to the snap store, before an actual write is commited to the physical disk.

Naturally, that snap store needs enough disk space to host all the changed pages. The safest way to ensure that a snap store has enough space is to attach a disk of equal size to the system, and make that available as a destination for CoW pages. This is rarely realistic, and in most cases, not needed. Even on busy disks, you will rarely overwrite the entire disk, by the time you finish the backup and dispose of the snapshot.

The kernel module gives you the ability to create a pipe through which you can send commands to manage snap stores and receive notifications for events such as:

  * Snap store almost full
  * Snap store is overflown
  * snap store has been deleted

In the event that the agent is stopped before the snap store is deleted, the kernel module returns a restartable system error (ERESTARTSYS), which allows the agent to re-attach itself to the existing snap stores, by simply executing the initiate command with the same arguments.