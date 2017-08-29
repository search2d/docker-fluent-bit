package main

import (
	"C"
	"errors"
	"log"
	"reflect"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/ugorji/go/codec"
)

type cloudWatchLogger struct {
	client        *cloudwatchlogs.CloudWatchLogs
	messageKey    string
	logGroupName  string
	logStreamName string
	sequenceToken *string
}

func (c *cloudWatchLogger) send(events []*cloudwatchlogs.InputLogEvent) error {
	if err := c.init(); err != nil {
		return err
	}

	out, err := c.client.PutLogEvents(&cloudwatchlogs.PutLogEventsInput{
		LogEvents:     events,
		LogGroupName:  aws.String(c.logGroupName),
		LogStreamName: aws.String(c.logStreamName),
		SequenceToken: c.sequenceToken,
	})
	if err != nil {
		return err
	}

	c.sequenceToken = out.NextSequenceToken

	return nil
}

func (c *cloudWatchLogger) init() error {
	if c.sequenceToken != nil {
		return nil
	}

	out, err := c.client.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(c.logGroupName),
		LogStreamNamePrefix: aws.String(c.logStreamName),
	})
	if err != nil {
		return err
	}

	for _, stream := range out.LogStreams {
		if *stream.LogStreamName == c.logStreamName {
			c.sequenceToken = stream.UploadSequenceToken
			return nil
		}
	}

	return errors.New("stream not found")
}

var cwl *cloudWatchLogger

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "cwlout", "CloudWatch Logs")
}

//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	messageKey := output.FLBPluginConfigKey(ctx, "MessageKey")
	if len(messageKey) == 0 {
		log.Printf("'MessageKey' entry required")
	}

	logGroupName := output.FLBPluginConfigKey(ctx, "LogGroupName")
	if len(logGroupName) == 0 {
		log.Printf("'LogGroupName' entry required")
	}

	logStreamName := output.FLBPluginConfigKey(ctx, "LogStreamName")
	if len(logStreamName) == 0 {
		log.Printf("'LogStreamName' entry required")
	}

	ses, err := session.NewSession()
	if err != nil {
		log.Print(err)
		return output.FLB_ERROR
	}

	cwl = &cloudWatchLogger{
		client:        cloudwatchlogs.New(ses),
		messageKey:    messageKey,
		logGroupName:  logGroupName,
		logStreamName: logStreamName,
	}

	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	var events []*cloudwatchlogs.InputLogEvent

	dec := codec.NewDecoderBytes(
		C.GoBytes(data, length),
		new(codec.MsgpackHandle),
	)

	for {
		var m interface{}
		if err := dec.Decode(&m); err != nil {
			break
		}
		slice := reflect.ValueOf(m)

		timestamp := int64(slice.Index(0).Interface().(uint64))

		for k, v := range slice.Index(1).Interface().(map[interface{}]interface{}) {
			if k.(string) == cwl.messageKey {
				buf, ok := v.([]uint8)
				if !ok {
					log.Print("invalid message type")
					break
				}
				msg := string(buf)

				events = append(events, &cloudwatchlogs.InputLogEvent{
					Message:   aws.String(msg),
					Timestamp: aws.Int64(timestamp * 1000),
				})

				break
			}
		}
	}

	if len(events) == 0 {
		return output.FLB_OK
	}

	if err := cwl.send(events); err != nil {
		log.Print(err)
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
