package main

import (
	"C"
	"fmt"
	"log"
	"reflect"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/ugorji/go/codec"
)

type cwlStream struct {
	client        *cloudwatchlogs.CloudWatchLogs
	events        []*cloudwatchlogs.InputLogEvent
	logGroupName  string
	logStreamName string
	sequenceToken *string
}

func (s *cwlStream) add(e *cloudwatchlogs.InputLogEvent) {
	s.events = append(s.events, e)
}

func (s *cwlStream) flush() error {
	log.Printf("%s flush", s.logStreamName)

	if len(s.events) == 0 {
		return nil
	}

	defer func() {
		s.events = nil
	}()

	if err := s.init(); err != nil {
		return err
	}

	out, err := s.client.PutLogEvents(&cloudwatchlogs.PutLogEventsInput{
		LogEvents:     s.events,
		LogGroupName:  aws.String(s.logGroupName),
		LogStreamName: aws.String(s.logStreamName),
		SequenceToken: s.sequenceToken,
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == cloudwatchlogs.ErrCodeInvalidSequenceTokenException {
				s.sequenceToken = nil
			}
		}
		return err
	}

	s.sequenceToken = out.NextSequenceToken

	return nil
}

func (s *cwlStream) init() error {
	if s.sequenceToken != nil {
		return nil
	}

retry:
	out, err := s.client.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(s.logGroupName),
		LogStreamNamePrefix: aws.String(s.logStreamName),
	})
	if err != nil {
		return err
	}

	for _, stream := range out.LogStreams {
		if *stream.LogStreamName == s.logStreamName {
			s.sequenceToken = stream.UploadSequenceToken
			return nil
		}
	}

	if _, err := s.client.CreateLogStream(&cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(s.logGroupName),
		LogStreamName: aws.String(s.logStreamName),
	}); err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == cloudwatchlogs.ErrCodeResourceAlreadyExistsException {
				log.Printf("%s: log stream already exists", s.logStreamName)
				goto retry
			}
		}
		return err
	}

	return nil
}

type cwlLogger struct {
	client  *cloudwatchlogs.CloudWatchLogs
	streams map[string]*cwlStream
}

func (l *cwlLogger) add(lgn string, lsn string, e *cloudwatchlogs.InputLogEvent) {
	key := lgn + ":" + lsn

	if _, exists := l.streams[key]; !exists {
		l.streams[key] = &cwlStream{
			client:        l.client,
			logGroupName:  lgn,
			logStreamName: lsn,
		}
	}

	l.streams[key].add(e)
}

func (l *cwlLogger) flush() multiError {
	errs := multiError{}

	for _, s := range l.streams {
		if err := s.flush(); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

type multiError []error

func (e multiError) occurred() bool {
	return 0 < len(e)
}

type eventParser struct {
	msgKey string
	lgnKey string
	lsnKey string
}

func (p *eventParser) parse(v interface{}) (string, string, *cloudwatchlogs.InputLogEvent, error) {
	timestamp, record := parseFluentdEvent(v)

	msg, err := record.string(p.msgKey)
	if err != nil {
		return "", "", nil, err
	}

	lgn, err := record.string(p.lgnKey)
	if err != nil {
		return "", "", nil, err
	}

	lsn, err := record.string(p.lsnKey)
	if err != nil {
		return "", "", nil, err
	}

	evt := &cloudwatchlogs.InputLogEvent{
		Message:   aws.String(msg),
		Timestamp: aws.Int64(int64(timestamp * 1000)),
	}

	return lgn, lsn, evt, nil
}

func parseFluentdEvent(v interface{}) (uint64, attrs) {
	slice := reflect.ValueOf(v)

	time := slice.Index(0).Interface().(uint64)
	data := slice.Index(1).Interface().(map[interface{}]interface{})

	return time, newAttrs(data)
}

type attrs map[string]interface{}

func newAttrs(src map[interface{}]interface{}) attrs {
	dst := attrs{}

	for k, v := range src {
		switch t := v.(type) {
		case map[interface{}]interface{}:
			dst[k.(string)] = newAttrs(t)
		case []byte:
			dst[k.(string)] = string(t)
		default:
			dst[k.(string)] = t
		}
	}

	return dst
}

func (a attrs) string(key string) (string, error) {
	val, ok := a[key]
	if !ok {
		return "", fmt.Errorf("'%s' attribute not found", key)
	}

	res, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("'%s' attribute should be string", key)
	}

	return res, nil
}

var logger *cwlLogger
var parser *eventParser

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "cwl", "CloudWatch Logs")
}

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	msgKey := output.FLBPluginConfigKey(ctx, "MessageKey")
	if len(msgKey) == 0 {
		log.Print("'MessageKey' entry required")
		return output.FLB_ERROR
	}

	lgnKey := output.FLBPluginConfigKey(ctx, "LogGroupNameKey")
	if len(lgnKey) == 0 {
		log.Print("'LogGroupNameKey' entry required")
		return output.FLB_ERROR
	}

	lsnKey := output.FLBPluginConfigKey(ctx, "LogStreamNameKey")
	if len(lsnKey) == 0 {
		log.Print("'LogStreamNameKey' entry required")
		return output.FLB_ERROR
	}

	parser = &eventParser{
		msgKey: msgKey,
		lgnKey: lgnKey,
		lsnKey: lsnKey,
	}

	ses, err := session.NewSession()
	if err != nil {
		log.Print(err)
		return output.FLB_ERROR
	}

	logger = &cwlLogger{
		client:  cloudwatchlogs.New(ses),
		streams: map[string]*cwlStream{},
	}

	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	dec := codec.NewDecoderBytes(
		C.GoBytes(data, length),
		new(codec.MsgpackHandle),
	)

	for {
		var v interface{}
		if err := dec.Decode(&v); err != nil {
			break
		}

		lgn, lsn, ev, err := parser.parse(v)
		if err != nil {
			log.Print(err)
			continue
		}

		logger.add(lgn, lsn, ev)
	}

	if errs := logger.flush(); errs.occurred() {
		for _, err := range errs {
			log.Print(err)
		}
		return output.FLB_ERROR
	}

	return output.FLB_OK
}

//export FLBPluginExit
func FLBPluginExit() int {
	return 0
}

func main() {
}
