#!/bin/bash
CURL=/usr/bin/curl
PUSHBULLET_TITLE=Raspberry\ Pi

LANG=ja_JP.utf8

${CURL} -u $1: -X POST \
    https://api.pushbullet.com/v2/pushes \
        --header "Content-Type: application/json" \
            --data-binary "{\"type\": \"note\", \"title\": \"${PUSHBULLET_TITLE}\", \"body\": \"$2\"}"