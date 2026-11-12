#!/bin/bash


COMMAND=${1:-help}


RANGE_START=3000
RANGE_END=3004



if [[ $COMMAND == "help" ]]; then
    echo "Usage: $0 [COMMAND]"
    echo "Commands:"
    echo "  help     - Show this help message."
    echo "  create   - Create VMs."
    echo "  destroy  - Destroy VMs."
    echo "  start    - Start VMs."
    echo "  stop     - Stop VMs."
    echo "  status   - Show VMs status."
    echo "  snapshot  - Create VMs snapshots."
    exit 0
fi

if [[ $COMMAND == "destroy" ]]; then
    echo "Destroying VMs..."

    for id in $(seq $RANGE_START $RANGE_END); do
        echo $id
        ssh root@prox-k12 "qm stop $id; qm destroy $id"
    done
fi

if [[ $COMMAND == "create" ]]; then
    echo "Creating VMs..."

    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm clone 9010 $id --name "k8s-$id" --full 0; qm start $id"
    done

    sleep 120

    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm guest exec $id -- truncate -s 0 /etc/machine-id"
        ssh root@prox-k12 "qm guest exec $id -- dbus-uuidgen --ensure=/etc/machine-id"

        # Clear the old DHCP lease files (example for netplan/systemd)
        ssh root@prox-k12 "qm guest exec $id -- rm /var/lib/dhcp/*"

        # Restart your network or reboot the VM
        ssh root@prox-k12 "qm guest exec $id -- reboot"
    done

    sleep 60

    for id in $(seq $RANGE_START $RANGE_END); do
	    ssh root@prox-k12 "qm agent $id network-get-interfaces | jq -r '.[1]."ip-addresses".[]."ip-address"' | grep 254"
    done

fi

if [[ $COMMAND == "status" ]]; then
    # echo "VMs status:"
    # for id in $(seq $RANGE_START $RANGE_END); do
    #     ssh root@prox-k12 "qm status $id"
    # done
    mapfile -t IPS < <(
    for id in $(seq "$RANGE_START" "$RANGE_END"); do
        ssh root@prox-k12 \
        "qm agent $id network-get-interfaces | jq -r '.[1].\"ip-addresses\".[].\"ip-address\"' | grep 254"
    done
    )


    CONTROL_PLANE_IPS=$(IFS=,; echo "${IPS[*]:0:3}")
    WORKER_NODE_IPS=$(IFS=,; echo "${IPS[*]:3}")

    echo "controller_ips=$CONTROL_PLANE_IPS worker_ips=$WORKER_NODE_IPS"
fi



if [[ $COMMAND == "stop" ]]; then
    echo "Stopping VMs..."
    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm stop $id"
    done
fi

if [[ $COMMAND == "start" ]]; then
    echo "Starting VMs..."
    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm start $id"
    done
fi

if [[ $COMMAND == "snapshot" ]]; then
    echo "Creating snapshots..."
    SNAPNAME=${2:-"snapshot"}
    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm snapshot $id $SNAPNAME"
    done
fi

if [[ $COMMAND == "freeze" ]]; then
    echo "Freezing VMs..."
    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm suspend $id --todisk"
    done
fi

if [[ $COMMAND == "unfreeze" ]]; then
    echo "Unfreezing VMs..."
    for id in $(seq $RANGE_START $RANGE_END); do
        ssh root@prox-k12 "qm resume $id"
    done
fi

