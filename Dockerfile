FROM golang:1.8.3-jessie AS build-env
RUN go get -u github.com/golang/dep/cmd/dep
COPY . /go/src/github.com/search2d/docker-fluent-bit
RUN cd /go/src/github.com/search2d/docker-fluent-bit && dep ensure -v
RUN cd /go/src/github.com/search2d/docker-fluent-bit && go build -buildmode=c-shared -o out_cwlout.so .

FROM fluent/fluent-bit:0.11.17
COPY --from=build-env /go/src/github.com/search2d/docker-fluent-bit/out_cwlout.so /fluent-bit/lib/out_cwlout.so
COPY fluent-bit.conf /fluent-bit/etc/fluent-bit.conf
ENV SERVICE_FLUSH=1
ENV SERVICE_LOG_LEVEL=info
ENV CWLOUT_MESSAGE_KEY=message
ENV CWLOUT_LOG_GROUP_NAME_KEY=log_group_name
ENV CWLOUT_LOG_STREAM_NAME_KEY=log_stream_name
CMD ["/fluent-bit/bin/fluent-bit", "-e", "/fluent-bit/lib/out_cwlout.so", "-c", "/fluent-bit/etc/fluent-bit.conf"]