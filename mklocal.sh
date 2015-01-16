#!/bin/bash

#sudo yum install golang mercurial

rm -rf $(pwd)/go
mkdir -vp $(pwd)/go/{bin,pkg,src}
export GOPATH=$(pwd)/go
go get code.google.com/p/redwood-filter
go install code.google.com/p/redwood-filter

