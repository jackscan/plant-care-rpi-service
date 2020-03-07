#!/bin/sh -e

dir=$1
cd $dir/..
/opt/bin/drive push -verbose -files -no-prompt -no-clobber plant
rm plant/*.jpg
