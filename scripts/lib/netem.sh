# Network delay between the worker box and the store box, for E7 and E8. Source after exp.env.
#
# Two boxes: netem on the worker box's egress interface, matching packets to $STORE_HOST
#   (optionally one destination port). Only worker -> store packets are delayed, so the
#   added round-trip time equals the netem delay.
# One box: netem on lo, matching the store ports in both directions, so every round trip
#   crosses it twice and the added round-trip time is twice the netem delay.
# Other traffic is never touched.

netem_dev() {
  if [ "$STORE_HOST" = localhost ]; then echo lo
  else ip route get "$STORE_HOST" | awk '{for (i = 1; i < NF; i++) if ($i == "dev") {print $(i + 1); exit}}'
  fi
}

netem_off() {
  tc qdisc del dev "$(netem_dev)" root 2>/dev/null || true
}

# netem_on "<netem parameters>" [port ...]   no ports = all traffic to the store box
netem_on() {
  local params=$1; shift
  local dev; dev=$(netem_dev)
  netem_off
  tc qdisc add dev "$dev" root handle 1: prio bands 4 priomap 1 2 2 2 1 2 0 0 1 1 1 1 1 1 1 1
  # shellcheck disable=SC2086
  tc qdisc add dev "$dev" parent 1:4 handle 40: netem $params
  if [ "$dev" = lo ]; then
    local ports=("$@"); [ ${#ports[@]} -eq 0 ] && ports=("$FS_PORT" "$REGISTRY_PORT")
    for p in "${ports[@]}"; do
      tc filter add dev lo parent 1: protocol ip prio 1 u32 match ip dport "$p" 0xffff flowid 1:4
      tc filter add dev lo parent 1: protocol ip prio 1 u32 match ip sport "$p" 0xffff flowid 1:4
    done
  elif [ $# -eq 0 ]; then
    tc filter add dev "$dev" parent 1: protocol ip prio 1 u32 match ip dst "$STORE_HOST/32" flowid 1:4
  else
    for p in "$@"; do
      tc filter add dev "$dev" parent 1: protocol ip prio 1 u32 \
        match ip dst "$STORE_HOST/32" match ip dport "$p" 0xffff flowid 1:4
    done
  fi
}

# netem_rtt <ms>   adds <ms> of round-trip time to all worker <-> store traffic
netem_rtt() {
  local ms=$1
  [ "$ms" = 0 ] && { netem_off; return; }
  if [ "$(netem_dev)" = lo ]; then
    netem_on "delay $(echo "$ms / 2" | bc -l | sed 's/0*$//;s/\.$//')ms"
  else
    netem_on "delay ${ms}ms"
  fi
}
