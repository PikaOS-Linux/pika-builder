#!/bin/bash
mkdir -p ./.tmp
cp ./users.json ./.tmp
cp ./config.json ./.tmp
templ generate && go build -o ./.tmp/gowebly_fiber .