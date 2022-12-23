#!/bin/bash
set -e

BASE_DIR=$(dirname "$(readlink -f "$0")")
DEFAULT_AGENT_SERVICE_PATH=/etc/systemd/system/coriolis-snapshot-agent.service
DEFAULT_AGENT_USERNAME=coriolis
DEFAULT_BINARY_PATH=/usr/local/bin/coriolis-snapshot-agent
DEFAULT_CONFIG_DIR=/etc/coriolis-snapshot-agent
DEFAULT_LOG_DIR=/var/log/coriolis
DEFAULT_SNAPSTORE_LOCATION=/mnt/snapstores/snapstore_files
CERTS_DIR=$DEFAULT_CONFIG_DIR/certs

MODULES_PATH=/etc/modules-load.d/veeamsnap.conf
PREREQS="gcc git make tar util-linux wget"

SNAPSTORE_WARNING=$(cat <<EOP
WARNING: The snapstore disk is not mounted.

Coriolis Snapshot Agent requires an extra disk device attached to this server, where it could store the created snapshots.
This mounted disk device is called a snapstore location.
The extra disk device can be a physical disk, a removable disk or an iSCSI attached disk.
Please make sure the server has such a disk device attached and mounted to $DEFAULT_SNAPSTORE_LOCATION, and rerun the installation process.
EOP
)

STEP_VERSION="0.19.0"
STEP_CLI_URL=https://github.com/smallstep/cli/releases/download/v${STEP_VERSION}/step_linux_${STEP_VERSION}_amd64.tar.gz

VEEAMSNAP_MODULE_PATH=/lib/modules/$(uname -r)/kernel/drivers/veeam/veeamsnap.ko
VEEAMSNAP_REPO_URL=https://github.com/cloudbase/veeamsnap
VEEAMSNAP_UDEV_RULE_FILEPATH=/etc/udev/rules.d/99-veeamsnap.rules

usage() {
cat << EOF
Usage ./setup.sh -H <CA_HOST> -f <CA_FINGERPRINT> [-phw]
Install Coriolis Snapshot Agent

-h          Display help

-H          Hostname of Step CA server

-f          Fingerprint of Step CA root certificate

-p          Port of Step CA server. Defaults to 9000

-w          Custom Web Root Directory Path. Defaults to '/var/www/html'.
            Only set this in case you install alongside a running web server.

-e          Current path to the coriolis-snapshot-agent executable
EOF
}

install_step_cli() {
    wget -O /tmp/step.tar.gz $STEP_CLI_URL || wget -O /tmp/step.tar.gz $STEP_CLI_URL --no-check-certificate
    tar -xf /tmp/step.tar.gz -C /tmp
    cp /tmp/step_$STEP_VERSION/bin/step /usr/bin
}

install_prereqs_suse() {
    KERNEL_TYPE=$(uname -r | cut -f 3 -d -)
    KERNEL_VERSION=$(uname -r | cut -f 1,2 -d -)
    VERSION=$(zypper search -si kernel-$KERNEL_TYPE | grep $KERNEL_VERSION | awk '{print $7}')
    zypper install -y $PREREQS gcc11 gettext-tools iproute2 kernel-$KERNEL_TYPE-devel-$VERSION
}

install_prereqs() {
    apt-get update && apt-get install -y $PREREQS gettext-base iproute2 linux-headers-$(uname -r) || true
    yum install -y $PREREQS gettext iproute kernel-devel-$(uname -r) || true
    install_prereqs_suse || true
    install_step_cli
}

build_veeamsnap() {
    git clone $VEEAMSNAP_REPO_URL
    cd ./veeamsnap/source
    # make
    make install
}

setup_veeamsnap() {
    grep -q veeamsnap $MODULES_PATH || echo veeamsnap > $MODULES_PATH
    touch $VEEAMSNAP_UDEV_RULE_FILEPATH
    echo 'KERNEL=="veeamsnap", OWNER="root", GROUP="disk"' > $VEEAMSNAP_UDEV_RULE_FILEPATH
    modprobe veeamsnap
    chgrp disk /dev/veeamsnap
}

copy_agent_binary() {
    mkdir -p $(dirname "$DEFAULT_BINARY_PATH")
    if [ "$1" != "$DEFAUL_BINARY_PATH" ]; then
        cp $1 $DEFAULT_BINARY_PATH
    fi

    # setting caps required to set kernel entry addresses from /proc/kallsyms
    setcap 'CAP_SYSLOG+ep' $DEFAULT_BINARY_PATH
}

render_config_file() {
    mkdir -p $DEFAULT_CONFIG_DIR
    mkdir -p $DEFAULT_LOG_DIR
    export DEFAULT_LOG_DIR
    DEFAULT_IP=$(ip -o route get 1 | sed -n 's/.*src \([0-9.]\+\).*/\1/p')
    echo "Which IP address should the snapshot agent service bind to?"
    read -p "[defaults to $DEFAULT_IP]: " BIND_ADDRESS
    BIND_ADDRESS=${BIND_ADDRESS:-$DEFAULT_IP}
    export BIND_ADDRESS

    echo "Which port should the snapshot agent service bind to?"
    read -p "[defaults to 9999]: " PORT
    PORT=${PORT:-9999}
    export PORT

    SNAPSTORE_DEVICE_UUID=$(blkid $SNAPSTORE_DEVICE -o export | grep ^UUID=)
    SNAPSTORE_DEVICE_PART_TYPE=$(blkid $SNAPSTORE_DEVICE -o value -s TYPE)
    if grep -q $SNAPSTORE_DEVICE_UUID /etc/fstab; then
        echo "Snapstore device already in fstab"
    else
        echo "Adding snapstore device to /etc/fstab"
        echo "$SNAPSTORE_DEVICE_UUID $DEFAULT_SNAPSTORE_LOCATION $SNAPSTORE_DEVICE_PART_TYPE defaults,nofail 0 0" >> /etc/fstab
    fi

    CONFIG_FILE_PATH=$DEFAULT_CONFIG_DIR/config.toml
    DISK_DEVICE_NAMES=$(lsblk -o NAME,SIZE,TYPE | grep -e 'disk$' | awk '{print $1}')
    SNAPSTORE_DISK=$(lsblk -dno pkname $SNAPSTORE_DEVICE)
    cat $BASE_DIR/config-template.toml | envsubst > $CONFIG_FILE_PATH
    for DISK_NAME in $DISK_DEVICE_NAMES; do
        # filter out disk set as snapstore
        if [ "$DISK_NAME" = "$SNAPSTORE_DISK" ]; then
            continue
        fi

        # filter out removable disks (i.e. floppy disks)
        if [ -f "/sys/block/$DISK_NAME/removable" ]; then
            if [ "$(cat /sys/block/$DISK_NAME/removable)" != "0" ]; then
                continue
            fi
        fi

        echo "[[snapstore_mapping]]" >> $CONFIG_FILE_PATH
        echo "device = \"$DISK_NAME\"" >> $CONFIG_FILE_PATH
        echo "location = \"$DEFAULT_SNAPSTORE_LOCATION\"" >> $CONFIG_FILE_PATH
    done
}

generate_certificates() {
    mkdir -p $CERTS_DIR
    step ca bootstrap -f --ca-url https://$1:$2 --fingerprint $3

    FLAG_ARGS="-f --provisioner=acme"
    if [ "$#" -eq "4" ]; then
        FLAG_ARGS="$FLAG_ARGS --webroot=$4"
    fi
    POS_ARGS="$BIND_ADDRESS $CERTS_DIR/srv-pub.pem $CERTS_DIR/srv-key.pem"
    step ca certificate $FLAG_ARGS $POS_ARGS

    step ca root -f $CERTS_DIR/ca-pub.pem
}

setup_service() {
    useradd --system --home-dir=/nonexisting --group disk --no-create-home --shell /bin/false $DEFAULT_AGENT_USERNAME

    chown $DEFAULT_AGENT_USERNAME:disk -R $DEFAULT_CONFIG_DIR
    chown $DEFAULT_AGENT_USERNAME:disk -R $DEFAULT_LOG_DIR
    chown $DEFAULT_AGENT_USERNAME:disk -R $DEFAULT_SNAPSTORE_LOCATION
    # Render service unit file
        #TODO: check if systemd or init
        # copy service unit file
    cp $BASE_DIR/coriolis-snapshot-agent.service.sample $DEFAULT_AGENT_SERVICE_PATH

    systemctl daemon-reload
    systemctl enable --now coriolis-snapshot-agent.service
    systemctl status coriolis-snapshot-agent.service
}

if [ "$EUID" -ne 0 ]; then
    echo "Installation script must be run with root privileges."
    exit 1
fi

SNAPSTORE_DEVICE=$(grep $DEFAULT_SNAPSTORE_LOCATION /proc/mounts | head -1 | awk '{print $1}')
if [ -z "$SNAPSTORE_DEVICE" ]; then
    echo "$SNAPSTORE_WARNING"
    exit 1
fi
echo "Identified snapstore device: $SNAPSTORE_DEVICE"

unset -v CA_HOST
unset -v CA_FINGERPRINT
unset -v CA_PORT
unset -v WEB_ROOT
unset -v EXEC_PATH

while getopts ":hH:f:p:w:e:" OPT; do
    case "$OPT" in
        h)
            usage
            exit 0
            ;;
        H) CA_HOST=$OPTARG ;;
        f) CA_FINGERPRINT=$OPTARG ;;
        p) CA_PORT=$OPTARG ;;
        w) WEB_ROOT=$OPTARG ;;
        e) EXEC_PATH=$OPTARG ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "$CA_HOST" ]; then
    read -p "Pass Hostname of the Step CA server (usually corresponds with the Coriolis appliance's hostname or address): " CA_HOST
fi

if [ -z "$CA_FINGERPRINT" ]; then
    read -p "Pass Step CA fingerprint (usually copied from the Coriolis WebUI, 'Coriolis Bare Metal Servers' tab): " CA_FINGERPRINT
fi

if [ -z "$EXEC_PATH" ]; then
    EXEC_PATH="$BASE_DIR/coriolis-snapshot-agent"
fi

if [ -z "$CA_PORT" ]; then
    CA_PORT=9000
fi

if [ -z "$WEB_ROOT" ]; then
    read -p "Is this machine a web server host? [y/n] " CONFIRMATION
    if [ "$CONFIRMATION" = "y" ]; then
        echo "In case this machine is a web server host, SSL certificate generation will require the web server's root, used to host ACME requests."
        read -p "Please provide the web server's root path [defaults to /var/www/html]: " WEB_ROOT
        WEB_ROOT=${WEB_ROOT:-"/var/www/html"}
    fi
fi

install_prereqs
if ! [[ -f $VEEAMSNAP_MODULE_PATH ]] ; then
    CLONEDIR=$(mktemp -d)
    pushd $CLONEDIR
    build_veeamsnap
    popd
    rm -r $CLONEDIR
fi
setup_veeamsnap

copy_agent_binary $EXEC_PATH
render_config_file
generate_certificates $CA_HOST $CA_PORT $CA_FINGERPRINT $WEB_ROOT
setup_service

echo "DONE!"
