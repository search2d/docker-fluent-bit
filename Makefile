run:
	docker run --rm \
	-p 24224:24224 \
	-v `pwd`:/data \
	--env-file .env \
	fluent/fluent-bit:0.11.17 \
	/fluent-bit/bin/fluent-bit -e /data/out_cwl.so -c /data/fluent-bit.conf

build:
	docker run --rm \
	-v `pwd`:/go/src/github.com/search2d/docker-fluent-bit \
	-w /go/src/github.com/search2d/docker-fluent-bit \
	golang:1.8.3-jessie \
	go build -buildmode=c-shared -o out_cwl.so .