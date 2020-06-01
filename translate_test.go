// Copyright © 2020 Xavier Basty <xavier@hexbee.net>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

//
//func TestMain(m *testing.M) {
//	go func() {
//		server := NewServer()
//		server.Run(4670)
//	}()
//
//	m.Run()
//}

func NewTranslate(t *testing.T) *translate.Translate {
	t.Helper()
	awsSession := session.Must(session.NewSession(&aws.Config{
		Endpoint: aws.String("http://localhost:4670"),
		Region:   aws.String("eu-west-1"),
	}))
	translateSvc := translate.New(awsSession)
	return translateSvc
}

////////////////////////////////////////////////////////////////////////////////

func TestDeleteTerminology(t *testing.T) {}

func TestDescribeTextTranslationJob(t *testing.T) {}

func TestGetTerminology(t *testing.T) {}

func TestImportTerminology(t *testing.T) {}

func TestListTerminologies(t *testing.T) {
	translateSvc := NewTranslate(t)

	output, err := translateSvc.ListTerminologies(&translate.ListTerminologiesInput{
		MaxResults: aws.Int64(10),
		NextToken:  aws.String("test-token"),
	})

	assert.NotNil(t, output)
	assert.NoError(t, err)
}

func TestListTextTranslationJobs(t *testing.T) {
	translateSvc := NewTranslate(t)

	output, err := translateSvc.ListTextTranslationJobs(&translate.ListTextTranslationJobsInput{
		Filter: &translate.TextTranslationJobFilter{
			JobName:             aws.String("test-job"),
			JobStatus:           aws.String(translate.JobStatusCompleted),
			SubmittedAfterTime:  aws.Time(time.Date(1985, 10, 26, 21, 0, 0, 0, time.UTC)),
			SubmittedBeforeTime: aws.Time(time.Date(2015, 10, 21, 16, 29, 0, 0, time.UTC)),
		},
		MaxResults: aws.Int64(10),
		NextToken:  aws.String("test-token"),
	})

	assert.NotNil(t, output)
	assert.NoError(t, err)
}

func TestStartTextTranslationJob(t *testing.T) {
	translateSvc := NewTranslate(t)
	output, err := translateSvc.StartTextTranslationJob(&translate.StartTextTranslationJobInput{
		DataAccessRoleArn: aws.String("arn:aws:iam::123456789012:role/test-role"),
		InputDataConfig: &translate.InputDataConfig{
			ContentType: aws.String("text/html"),
			S3Uri:       aws.String("s3://input-bucket/input-key"),
		},
		JobName: aws.String("test-job"),
		OutputDataConfig: &translate.OutputDataConfig{
			S3Uri: aws.String("s3://output-bucket/output-key"),
		},
		SourceLanguageCode:  aws.String("en"),
		TargetLanguageCodes: aws.StringSlice([]string{"fr"}),
		TerminologyNames:    []*string{aws.String("test-terminology")},
	})

	assert.NotNil(t, output)
	assert.NoError(t, err)
	spew.Dump(output)
}

func TestStopTextTranslationJob(t *testing.T) {
	t.Run("make sure there's a job running", TestStartTextTranslationJob)

	translateSvc := NewTranslate(t)
	output, err := translateSvc.StopTextTranslationJob(&translate.StopTextTranslationJobInput{
		JobId: aws.String("0"),
	})

	assert.NotNil(t, output)
	assert.NoError(t, err)
}

func TestTranslateText(t *testing.T) {
	translateSvc := NewTranslate(t)

	output, err := translateSvc.Text(&translate.TextInput{
		SourceLanguageCode: aws.String("en"),
		TargetLanguageCode: aws.String("fr"),
		TerminologyNames:   []*string{aws.String("test=terminology")},
		Text:               aws.String("test input text"),
	})

	assert.NotNil(t, output)
	assert.NoError(t, err)
}
