# go-kvstore
Simple KV store implementation in Go

## Reference

https://build-your-own.org/database/

## Usage

curl -X PUT "http://localhost:8080/kv?key=name" \
     -H "Content-Type: application/json" \
     -d '{"value":"name1"}'

curl "http://localhost:8080/kv?key=name"

curl -X DELETE "http://localhost:8080/kv?key=name"