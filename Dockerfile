FROM golang:1.8.3-jessie AS build-env
RUN go get -u github.com/golang/dep/cmd/dep
COPY . /go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output
RUN cd /go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output && dep ensure -v
RUN cd /go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output && go build -buildmode=c-shared -o out_cwlout.so .

FROM fluent/fluent-bit:0.11.17
COPY --from=build-env /go/src/github.com/search2d/fluent-bit-cloudwatchlogs-output/out_cwlout.so /fluent-bit/lib/out_cwlout.so
COPY fluent-bit.conf /fluent-bit/etc/fluent-bit.conf
CMD ["/fluent-bit/bin/fluent-bit", "-e", "/fluent-bit/lib/out_cwlout.so", "-c", "/fluent-bit/etc/fluent-bit.conf"]