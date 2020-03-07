#!/bin/sh

# wait 3 minutes (18*10s) for clock to be at most 10min off
/usr/bin/chronyc waitsync 18 10
