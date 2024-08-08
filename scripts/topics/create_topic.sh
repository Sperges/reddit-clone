#!/bin/bash
curl 127.0.0.1:9001/topics --header "Content-Type: application/json" --data '{"model": {"ID": "test"}}'