#!/bin/bash
set -e

BASE_DIR=$(dirname "$(readlink -f "$0")")
DEFAULT_AGENT_SERVICE_PATH=/etc/systemd/system/coriolis-snapshot-agent.service
DEFAULT_AGENT_USERNAME=coriolis
DEFAULT_BINARY_PATH=/usr/local/bin/coriolis-snapshot-agent
DEFAULT_CONFIG_DIR=/etc/coriolis-snapshot-agent
DEFAULT_SNAPSTORE_LOCATION=/mnt/snapstores/snapstore_files
CERTS_DIR=$DEFAULT_CONFIG_DIR/certs

MODULES_PATH=/etc/modules
PREREQS="gcc git make tar wget"

STEP_VERSION="0.19.0"
STEP_CLI_URL=https://dl.step.sm/gh-release/cli/docs-ca-install/v$STEP_VERSION/step_linux_${STEP_VERSION}_amd64.tar.gz

VEEAMSNAP_MODULE_PATH=/lib/modules/$(uname -r)/kernel/drivers/veeam/veeamsnap.ko
VEEAMSNAP_REPO_URL=https://github.com/veeam/veeamsnap
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
EOF
}

install_step_cli() {
    wget -O /tmp/step.tar.gz $STEP_CLI_URL
    tar -xf /tmp/step.tar.gz -C /tmp
    cp /tmp/step_$STEP_VERSION/bin/step /usr/bin
}

install_prereqs() {
    apt-get update || true
    apt-get install -y $PREREQS gettext-base iproute2 linux-headers-$(uname -r) || yum install -y $PREREQS gettext iproute kernel-devel-$(uname -r)
    install_step_cli
}

build_veeamsnap() {
    pushd /tmp
    git clone $VEEAMSNAP_REPO_URL
    cd /tmp/veeamsnap/source
    # make
    make install

    echo veeamsnap >> $MODULES_PATH
    touch $VEEAMSNAP_UDEV_RULE_FILEPATH
    echo 'KERNEL=="veeamsnap", OWNER="root", GROUP="disk"' > $VEEAMSNAP_UDEV_RULE_FILEPATH
    modprobe veeamsnap
    popd
}

copy_agent_binary() {
    mkdir -p $(dirname "$DEFAULT_BINARY_PATH")
    cp $BASE_DIR/coriolis-snapshot-agent $DEFAULT_BINARY_PATH
}

make_dirs() {
    mkdir -p $CERTS_DIR
    mkdir -p $DEFAULT_SNAPSTORE_LOCATION
}

render_config_file() {
    DEFAULT_IP=$(ip -o route get 1 | sed -n 's/.*src \([0-9.]\+\).*/\1/p')
    echo "Which IP address should the snapshot agent service bind to?"
    read -p "[defaults to $DEFAULT_IP]: " BIND_ADDRESS
    BIND_ADDRESS=${BIND_ADDRESS:-$DEFAULT_IP}
    export BIND_ADDRESS

    echo "Which port should the snapshot agent service bind to?"
    read -p "[defaults to 9999]: " PORT
    PORT=${PORT:-9999}
    export PORT

    while : ; do
        echo "The snapshots created by the agent are saved in a snapstore location."
        echo "The snapstore location is the mount path of a separate empty block volume (physical, iSCSI, rbd, etc)."
        DISKS_LIST=$(lsblk -o NAME,SIZE,TYPE | grep -e 'disk$')
        echo "$DISKS_LIST" | awk '{print $1 "\t\t" $2}'
        DISK_DEVICE_NAMES=$(echo "$DISKS_LIST" | awk '{print $1}')
        echo
        read -p "Which disk will be used as snapstore? " SNAPSTORE_DISK
        if ! echo $DISK_DEVICE_NAMES | grep -q $SNAPSTORE_DISK; then
            echo "Selected disk does not exist: $SNAPSTORE_DISK"
            continue
        fi
        SNAPSTORE_DISK=/dev/$SNAPSTORE_DISK
        read -p "Do you wish to format the snapstore disk? (y/n) " SNAPSTORE_CONFIRMATION
        if [ "$SNAPSTORE_CONFIRMATION" = "y" ]; then
            umount $SNAPSTORE_DISK || true
            mkfs.ext4 $SNAPSTORE_DISK
        fi

        if grep -q $SNAPSTORE_DISK /proc/mounts; then
            echo "Snapstore disk already mounted."
            break
        else
            mount $SNAPSTORE_DISK $DEFAULT_SNAPSTORE_LOCATION
            if grep -q $SNAPSTORE_DISK /proc/mounts; then
                echo "Snapstore disk mounted successfully"
            else
                echo "WARN: Could not mount disk $SNAPSTORE_DISK"
                continue
            fi
            break
        fi
    done

    CONFIG_FILE_PATH=$DEFAULT_CONFIG_DIR/config.toml
    cat $BASE_DIR/config-template.toml | envsubst > $CONFIG_FILE_PATH
    for DISK_NAME in $DISK_DEVICE_NAMES; do
        if [ "/dev/$DISK_NAME" = "$SNAPSTORE_DISK" ]; then
            continue
        fi

        echo "[[snapstore_mapping]]" >> $CONFIG_FILE_PATH
        echo "device = \"$DISK_NAME\"" >> $CONFIG_FILE_PATH
        echo "location = \"$DEFAULT_SNAPSTORE_LOCATION\"" >> $CONFIG_FILE_PATH
    done
}

generate_certificates() {
    step ca bootstrap -f --ca-url https://$1:$2 --fingerprint $3

    FLAG_ARGS="-f --provisioner=acme"
    POS_ARGS="$BIND_ADDRESS $CERTS_DIR/srv-pub.pem $CERTS_DIR/srv-key.pem"
    step ca certificate $FLAG_ARGS --webroot=$4 $POS_ARGS || step ca certificate $FLAG_ARGS $POS_ARGS

    step ca root -f $CERTS_DIR/ca-pub.pem
}

setup_service() {
    useradd --system --home-dir=/nonexisting --group disk --no-create-home --shell /bin/false $DEFAULT_AGENT_USERNAME

    chown $DEFAULT_AGENT_USERNAME:disk -R $DEFAULT_CONFIG_DIR
    chown $DEFAULT_AGENT_USERNAME:disk -R $DEFAULT_SNAPSTORE_LOCATION
    # Render service unit file
        #TODO: check if systemd or init
        # copy service unit file
    cp $BASE_DIR/coriolis-snapshot-agent.service.sample $DEFAULT_AGENT_SERVICE_PATH

    systemctl daemon-reload
    systemctl enable --now coriolis-snapshot-agent.service
}

unset -v CA_HOST
unset -v CA_FINGERPRINT
unset -v CA_PORT
unset -v WEB_ROOT

while getopts ":hH:f:p:w:" OPT; do
    case "$OPT" in
        h)
            usage
            exit 0
            ;;
        H) CA_HOST=$OPTARG ;;
        f) CA_FINGERPRINT=$OPTARG ;;
        p) CA_PORT=$OPTARG ;;
        w) WEB_ROOT=$OPTARG ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "$CA_HOST" ] || [ -z "$CA_FINGERPRINT" ]; then
    echo "Missing -H or -f option"
    exit 1
fi

if [ -z "$CA_PORT" ]; then
    CA_PORT=9000
fi

if [ -z "$WEB_ROOT" ]; then
    WEB_ROOT=/var/www/html
fi

install_prereqs
if ! [[ -f $VEEAMSNAP_MODULE_PATH ]] ; then
    build_veeamsnap
fi

copy_agent_binary
make_dirs
render_config_file
generate_certificates $CA_HOST $CA_PORT $CA_FINGERPRINT $WEB_ROOT
setup_service

echo "DONE!"
