#!/usr/bin/env bash
UDEV_RULES_FILE="${UDEV_RULES_FILE:-/etc/udev/rules.d/70-persistent-net.rules}"

exec 3>&1
log() {
  echo "$@" >&3
}

extract_mac_addresses() {
  local macs=()
  # Iterate through /sys/class/net to find MAC addresses
  for device in /sys/class/net/*; do
    if [[ -f "$device/address" ]]; then
      mac=$(cat "$device/address" 2>/dev/null)
      if [[ -n "$mac" ]]; then
        macs+=("$mac")
      fi
    fi
  done
  # Return the list of MAC addresses
  echo "${macs[@]}"
}

extract_ifconfig() {
  declare -A devices
  interfaces=$(ifconfig -a | grep -o "^[a-zA-Z0-9@]*" | grep -v "^$")
  log "Interfaces found: $interfaces"

  for interface in $interfaces; do
    mac=$(cat "/sys/class/net/$device/address" 2>/dev/null)
    log "Interface: $interface, MAC: $mac"
    if [[ -n "$mac" ]]; then
      devices["$interface"]="$mac"
    fi
  done
  log "Devices: ${devices[*]}"

  for device in "${!devices[@]}"; do
    log "  Device: $device, MAC: ${devices[$device]}"
  done

  for device in "${!devices[@]}"; do
    log "$device:${devices[$device]}"
    echo "$device:${devices[$device]}"
  done
}

extract_nm_connections() {
  declare -A devices
  local macs=("$@")
  local i=0
  for file in /etc/NetworkManager/system-connections/*; do
    if [[ -f "$file" ]]; then
      device=$(grep -oP '(?<=^interface-name=).*' "$file")
      log "File: $file, Device: $device"

      if [[ -n "$device" ]]; then
        mac=${macs[i]}
        log "Device: $device, MAC: $mac"

        if [[ -n "$mac" ]]; then
          devices["$device"]="$mac"
        fi
        i=$((i + 1))
      fi
    fi
  done

  log "Devices from NetworkManager: ${devices[*]}"

  for device in "${!devices[@]}"; do
    log "$device:${devices[$device]}"
    echo "$device:${devices[$device]}"
  done
}

extract_sysconfig_connections() {
  declare -A devices

  for file in /etc/sysconfig/network-scripts/ifcfg-*; do
    if [[ -f "$file" ]]; then
      # Extract the device name (DEVICE= value)
      device=$(grep -oP '(?<=^DEVICE=).*' "$file" | tr -d '"')
      log "File: $file, Device: $device"
      mac=$(grep -oP '(?<=^HWADDR=).*' "$file" | tr -d '"')
      if [[ -n "$device" && -n "$mac" ]]; then
        devices["$device"]="$mac"
      fi
    fi
  done
  log "Devices from sysconfig: ${devices[*]}"
  for device in "${!devices[@]}"; do
    log "$device:${devices[$device]}"
    echo "$device:${devices[$device]}"
  done
}

extract_netplan_connections() {
  declare -A devices
  local macs=("$@")
  local i=0
  for file in /etc/netplan/*.yaml; do
    if [[ -f "$file" ]]; then
      interface=$(awk '/ethernets:/{flag=1;next} flag && /^[[:space:]]+[a-zA-Z0-9_-]+:/{gsub(":", "", $1); print $1; flag=0}' "$file")
      log "Netplan file: $file, Extracted interface: $interface"
      if [[ -n "$interface" ]]; then
        mac=${macs[i]}
        log "Device: $interface, MAC: $mac"
        if [[ -n "$mac" ]]; then
          devices["$interface"]="$mac"
        fi
        i=$((i + 1))
      fi
    fi
  done

  log "Devices from netplan: ${devices[*]}"
  for device in "${!devices[@]}"; do
    log "$device:${devices[$device]}"
    echo "$device:${devices[$device]}"
  done
}

generate_udev_rules() {
  # shellcheck disable=SC2190
  local devices=("$@")
  echo "Generating udev rules..."
  echo "# Persistent network device rules" >"$UDEV_RULES_FILE"
  for entry in "${devices[@]}"; do
    # Parse the serialized string (device:MAC)
    device="${entry%%:*}"
    mac="${entry#*:*}"
    if [[ -n "$device" && -n "$mac" ]]; then
      echo "SUBSYSTEM==\"net\",ACTION==\"add\",ATTR{address}==\"$mac\",NAME=\"$device\"" >>"$UDEV_RULES_FILE"
    fi
  done
  echo "Udev rules written to $UDEV_RULES_FILE"
}

main() {
  mapfile -t macs < <(extract_mac_addresses)
  log "${macs[*]}"
  mapfile -t exdevices < <(extract_nm_connections "${macs[@]}")
  if [[ ${#exdevices[@]} -eq 0 ]]; then
    log "No network devices found in NetworkManager. Trying sysconfig..."
    mapfile -t exdevices < <(extract_sysconfig_connections)
    if [[ ${#exdevices[@]} -eq 0 ]]; then
      log "No network devices found in sysconfig. Trying netplan..."
      if [[ -d /etc/netplan ]]; then
        mapfile -t exdevices < <(extract_netplan_connections "${macs[@]}")
      fi
      if [[ ${#exdevices[@]} -eq 0 ]]; then
        log "No network devices found. Trying . Ifcfg..."
        mapfile -t exdevices < <(extract_ifconfig)
        if [[ ${#exdevices[@]} -eq 0 ]]; then
          log "No network devices... existing"
          exit 0
        fi
      fi
    fi
  fi
  generate_udev_rules "${exdevices[@]}"
}
main
