#!/bin/bash
curl -X POST 127.0.0.1:9001/topics/test/posts --header "Content-Type: application/json" --data '{"model": {"title": "Hello", "content": "World"}}'