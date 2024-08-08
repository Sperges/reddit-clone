#!/bin/bash
curl -X POST 127.0.0.1:9001/topics/test/posts/270712a3-90d0-43d1-a19e-769c1951ff0e/comments --header "Content-Type: application/json" --data '{"model": {"content": "haha nice"}}'