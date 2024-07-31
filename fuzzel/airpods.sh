#!/bin/bash

addr="74:8F:3C:A5:67:ED"

connected=$(bluetoothctl devices Connected)

if [[ $connected = *$addr* ]]; then
  echo "Already Connected"
else 
  bluetoothctl connect $addr
fi
