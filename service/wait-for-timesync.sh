#!/bin/sh

while /usr/bin/timedatectl status | /usr/bin/grep -e 'synchronized: no'
do
    /usr/bin/sleep 1
done
