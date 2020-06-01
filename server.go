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
	"encoding/json"
	"fmt"
	"github.com/apex/log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	opDeleteTerminology          = "DeleteTerminology"
	opDescribeTextTranslationJob = "DescribeTextTranslationJob"
	opGetTerminology             = "GetTerminology"
	opImportTerminology          = "ImportTerminology"
	opListTerminologies          = "ListTerminologies"
	opListTextTranslationJobs    = "ListTextTranslationJobs"
	opStartTextTranslationJob    = "StartTextTranslationJob"
	opStopTextTranslationJob     = "StopTextTranslationJob"
	opTranslateText              = "TranslateText"
)

type AWSRequest struct {
	Action  string
	Version string
	Date    time.Time
}

type Server struct {
	Config    Config
	mux       *http.ServeMux
	scheduler Scheduler
	throttler Throttler
}

func NewServer(config Config, jobs ...translate.TextTranslationJobProperties) *Server {
	s := Server{
		Config:    config,
		mux:       http.NewServeMux(),
		scheduler: NewJobScheduler(&config, jobs...),
		throttler: NewCallThrottler(&config),
	}

	s.mux.HandleFunc("/", s.HandleRequest)

	return &s
}

func (s *Server) Run(port int) {
	log.Infof("listening on :%v ...", port)
	log.Fatalf("error listening on :%v: %v", port, http.ListenAndServe(fmt.Sprintf(":%d", port), s.mux))
}

func (s *Server) HandleRequest(writer http.ResponseWriter, request *http.Request) {
	UpdateScheduler(s.scheduler)

	awsRequest, err := parseAWSHeaders(request)
	if err != nil {
		awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		return
	}
	log.Debugf("received action=%s, version=%s, date=%d", awsRequest.Action, awsRequest.Version, awsRequest.Date)

	switch awsRequest.Action {
	case opTranslateText:
		s.HandleAPICall(writer, request.Body, &translate.TextInput{}, s.HandleTranslateText)
	case opStartTextTranslationJob:
		s.HandleAPICall(writer, request.Body, &translate.StartTextTranslationJobInput{}, s.HandleStartTextTranslationJob)
	case opStopTextTranslationJob:
		s.HandleAPICall(writer, request.Body, &translate.StopTextTranslationJobInput{}, s.HandlerStopTextTranslationJob)
	case opListTextTranslationJobs:
		s.HandleAPICall(writer, request.Body, &translate.ListTextTranslationJobsInput{}, s.HandleListTextTranslationJobs)
	case opDescribeTextTranslationJob:
		s.HandleAPICall(writer, request.Body, &translate.DescribeTextTranslationJobInput{}, s.HandleDescribeTextTranslationJob)
	case opImportTerminology:
		s.HandleAPICall(writer, request.Body, &translate.ImportTerminologyInput{}, s.HandleImportTerminology)
	case opListTerminologies:
		s.HandleAPICall(writer, request.Body, &translate.ListTerminologiesInput{}, s.HandleListTerminologies)
	case opGetTerminology:
		s.HandleAPICall(writer, request.Body, &translate.GetTerminologyInput{}, s.HandleGetTerminology)
	case opDeleteTerminology:
		s.HandleAPICall(writer, request.Body, &translate.DeleteTerminologyInput{}, s.HandleDeleteTerminology)
	default:
		awsError(writer, errors.Errorf("unrecognized action: %s", awsRequest.Action), "InvalidAction", http.StatusBadRequest)
	}
}

////////////////////////////////////////////////////////////////////////////////

func (s *Server) HandleAPICall(writer http.ResponseWriter, reader io.Reader, input interface{}, handler func(writer http.ResponseWriter, input interface{})) {
	if s.throttler.CheckThrottling() {
		awsError(writer, errors.Errorf("number of calls exceeded %d", s.Config.ThrottlingThreshold), "TooManyRequestsException", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(input); err != nil {
		awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		return
	}

	s.throttler.PushCall()
	handler(writer, input)
}

func (s *Server) HandleTranslateText(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.TextInput)
	log.Info("handle: TranslateText")
	log.Debugf("TranslateText(%s)", spew.Sdump(input))

	terminologies := make([]*translate.AppliedTerminology, 0, len(input.TerminologyNames))

	for _, t := range input.TerminologyNames {
		var terms = []*translate.Term{
			{
				SourceText: aws.String("terminology-source"),
				TargetText: aws.String("terminology-target"),
			},
		}
		terminologies = append(terminologies, &translate.AppliedTerminology{
			Name:  t,
			Terms: terms,
		})
	}
	output := translate.TextOutput{
		AppliedTerminologies: terminologies,
		SourceLanguageCode:   input.SourceLanguageCode,
		TargetLanguageCode:   input.TargetLanguageCode,
		TranslatedText:       aws.String(fmt.Sprintf("translated: %s", aws.StringValue(input.Text))),
	}

	writeOutput(writer, output)
}

func (s *Server) HandleStartTextTranslationJob(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.StartTextTranslationJobInput)
	log.Info("handle: StartTextTranslationJob")
	log.Debugf("StartTextTranslationJob(%s)", spew.Sdump(input))

	now := time.Now()
	job := translate.TextTranslationJobProperties{
		JobName:             input.JobName,
		SourceLanguageCode:  input.SourceLanguageCode,
		TargetLanguageCodes: input.TargetLanguageCodes,
		DataAccessRoleArn:   input.DataAccessRoleArn,
		InputDataConfig:     input.InputDataConfig,
		OutputDataConfig:    input.OutputDataConfig,
		TerminologyNames:    input.TerminologyNames,
		JobStatus:           aws.String(translate.JobStatusSubmitted),
		SubmittedTime:       &now,
		EndTime:             nil,
		Message:             nil,
		JobDetails: &translate.JobDetails{
			DocumentsWithErrorsCount: aws.Int64(0),
			InputDocumentsCount:      aws.Int64(0),
			TranslatedDocumentsCount: aws.Int64(0),
		},
	}

	jobID := s.scheduler.AddJobStartRequest(&job, now)
	log.Infof("%s: job submitted", jobID)

	writeOutput(writer, translate.StartTextTranslationJobOutput{
		JobId:     job.JobId,
		JobStatus: job.JobStatus,
	})
}

func (s *Server) HandlerStopTextTranslationJob(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.StopTextTranslationJobInput)
	log.Info("action: StopTextTranslationJob")
	log.Debugf("StopTextTranslationJob(%s)", spew.Sdump(input))

	jobID := *input.JobId
	job, ok := s.scheduler.Jobs()[jobID]
	if !ok {
		awsError(writer, errors.Errorf("stop: job '%s' was not found", *input.JobId), "ResourceNotFoundException", http.StatusBadRequest)
		return
	}

	s.scheduler.AddJobStopRequest(job, time.Now())
	job.JobStatus = aws.String(translate.JobStatusStopRequested)

	writeOutput(writer, translate.StopTextTranslationJobOutput{
		JobId:     input.JobId,
		JobStatus: job.JobStatus,
	})
}

func (s *Server) HandleListTextTranslationJobs(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.ListTextTranslationJobsInput)
	log.Info("handle: ListTextTranslationJobs")
	log.Debugf("ListTextTranslationJobs(%s)", spew.Sdump(input))

	//TODO: use Filter, MaxResults and NextToken
	schedulerJobs := s.scheduler.Jobs()
	jobs := make([]*translate.TextTranslationJobProperties, 0, len(schedulerJobs))
	for _, job := range schedulerJobs {
		jobs = append(jobs, job)
	}

	//TODO: manage token
	output := translate.ListTextTranslationJobsOutput{
		NextToken:                        aws.String("test-token"),
		TextTranslationJobPropertiesList: jobs,
	}
	writeOutput(writer, output)
}

func (s *Server) HandleDescribeTextTranslationJob(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.DescribeTextTranslationJobInput)
	log.Info("handle: DescribeTextTranslationJob")
	log.Debugf("DescribeTextTranslationJob(%s)", spew.Sdump(input))

	jobID := *input.JobId
	job, ok := s.scheduler.Jobs()[jobID]
	if !ok {
		awsError(writer, errors.Errorf("describe: job '%s' was not found", jobID), "ResourceNotFoundException", http.StatusBadRequest)
		return
	}

	writeOutput(writer, *job)
}

func (s *Server) HandleImportTerminology(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.ImportTerminologyInput)
	log.Info("action: ImportTerminology")
	log.Debugf("ImportTerminology(%s)", spew.Sdump(input))

	output := translate.ImportTerminologyOutput{}
	writeOutput(writer, output)
}

func (s *Server) HandleListTerminologies(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.ListTerminologiesInput)
	log.Info("action: ListTerminologies")
	log.Debugf("ListTerminologies(%s)", spew.Sdump(input))

	//output := translate.ListTerminologiesOutput{}
	//writeOutput(writer, output)
}

func (s *Server) HandleGetTerminology(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.GetTerminologyInput)
	log.Info("action: GetTerminology")
	log.Debugf("GetTerminology(%s)", spew.Sdump(input))

	//output := translate.GetTerminologyOutput{}
	//writeOutput(writer, output)
}

func (s *Server) HandleDeleteTerminology(writer http.ResponseWriter, in interface{}) {
	input := in.(*translate.DeleteTerminologyInput)
	log.Info("action: DeleteTerminology")
	log.Debugf("DeleteTerminology(%s)", spew.Sdump(input))

	//output := translate.DeleteTerminologyOutput{}
	//writeOutput(writer, output)
}

////////////////////////////////////////////////////////////////////////////////

func parseAWSHeaders(request *http.Request) (*AWSRequest, error) {
	r := AWSRequest{}
	if date := request.Header.Get("X-Amz-Date"); date != "" {
		parsedDate, err := time.Parse("20060102T150405Z", date)
		if err != nil {
			log.WithError(err).Errorf("failed to parse X-Amz-Date: %s", date)
		}
		r.Date = parsedDate
	}

	if target := strings.Split(request.Header.Get("X-Amz-Target"), "."); len(target) == 2 {
		r.Action = target[1]
		if version := strings.Split(target[0], "_"); len(version) == 2 {
			r.Version = target[1]
		} else {
			return nil, errors.Errorf("invalid X-Amz-Target header: '%s'", request.Header.Get("X-Amz-Target"))
		}
	} else {
		return nil, errors.Errorf("invalid X-Amz-Target header: '%s'", request.Header.Get("X-Amz-Target"))
	}

	return &r, nil
}

func writeOutput(writer http.ResponseWriter, output interface{}) {
	js, err := json.Marshal(output)
	if err != nil {
		log.WithError(err).Error("failed to marshal output to json")
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err = writer.Write(js); err != nil {
		log.WithError(err).Error("failed to write to http writer")
	}
}

func awsError(writer http.ResponseWriter, err error, awsErr string, code int) {
	log.Error(err.Error())
	http.Error(writer, fmt.Sprintf(`{
    "__type": "%s"
}`, awsErr), code)
}
