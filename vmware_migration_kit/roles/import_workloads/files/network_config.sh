#!/bin/bash
UDEV_RULES_FILE="${UDEV_RULES_FILE:-/etc/udev/rules.d/70-persistent-net.rules}"

exec 3>&1
log() {
    echo $@ >&3
}

extract_ifconfig() {

  declare -A devices
  interfaces=$(ifconfig -a | grep -o "^[a-zA-Z0-9@]*" | grep -v "^$")
  log "Interfaces found: $interfaces"

  for interface in $interfaces; do
    mac=$(ifconfig "$interface" 2>/dev/null | grep -i "ether" | awk '{print $2}')
    log "Interface: $interface, MAC: $mac"
    if [[ -n "$mac" ]]; then
      devices["$interface"]="$mac"
    fi
  done
  log "Devices: ${devices}"

  for device in "${!devices[@]}"; do
    log "  Device: $device, MAC: ${devices[$device]}"
  done

  for device in "${!devices[@]}"; do
    log "$device:${devices[$device]}"
    echo "$device:${devices[$device]}"
  done
}

generate_udev_rules() {
  local devices=("$@")

  echo "Generating udev rules..."
  echo "# Persistent network device rules" > "$UDEV_RULES_FILE"

  for entry in "${devices[@]}"; do
    # Parse the serialized string (device:MAC)
    device="${entry%%:*}"
    mac="${entry#*:*}"
    if [[ -n "$device" && -n "$mac" ]]; then
      echo "SUBSYSTEM==\"net\",ACTION==\"add\",ATTR{address}==\"$mac\",NAME=\"$device\"" >> "$UDEV_RULES_FILE"
    fi
  done

  echo "Udev rules written to $UDEV_RULES_FILE"
}

main() {
  exdevices=($(extract_ifconfig))

  if [[ ${#exdevices[@]} -eq 0 ]]; then
    echo "No network devices found. Exiting."
    exit 1
  fi

  generate_udev_rules "${exdevices[@]}"
}
main
