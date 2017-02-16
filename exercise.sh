#!/bin/bash

set -ex

trap "exit 0" SIGINT SIGTERM

for agent in {1..10}; do
	IP=$(( ( RANDOM % 255 ) + 1 ))
	IP2=$(( ( RANDOM % 16 ) + 1 ))
	IP3=$(( ( RANDOM % 8 ) + 1 ))
	wrk -d 10 -c 10 -t 10 http://localhost:8888/cwp-installer/ -H "X-Forwarded-For: 1.$IP3.$IP2.$IP" &
done
