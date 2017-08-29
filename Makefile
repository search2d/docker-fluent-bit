run:
	docker run --rm \
	-p 24224:24224 \
	-v `pwd`:/data \
	--env-file .env \
	fluent/fluent-bit:0.11.17 \
	/fluent-bit/bin/fluent-bit -e /data/out_cwlout.so -c /data/out_cwlout.conf

build:
	docker run --rm \
	-v `pwd`:/go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output \
	-w /go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output \
	golang:1.8.3-jessie \
	go build -buildmode=c-shared -o out_cwlout.so .