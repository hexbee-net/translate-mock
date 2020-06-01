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
	"container/ring"
	"encoding/json"
	"fmt"
	"github.com/apex/log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"net/http"
	"strconv"
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
	Config Config

	mux   *http.ServeMux
	jobs  map[string]*translate.TextTranslationJobProperties
	calls *ring.Ring
}

func NewServer(config Config, jobs ...translate.TextTranslationJobProperties) *Server {
	s := Server{
		Config: config,
		mux:    http.NewServeMux(),
		jobs:   make(map[string]*translate.TextTranslationJobProperties),
		calls:  ring.New(config.ThrottlingThreshold),
	}

	for _, job := range jobs {
		s.jobs[*job.JobId] = &job
	}

	s.mux.HandleFunc("/", s.HandleRequest)

	return &s
}

func (s *Server) Run(port int) {
	log.Infof("listening on :%v ...", port)
	log.Fatalf("error listening on :%v: %v", port, http.ListenAndServe(fmt.Sprintf(":%d", port), s.mux))
}

func (s *Server) HandleRequest(writer http.ResponseWriter, request *http.Request) {
	s.UpdateJobs()
	if s.checkThrottling() {
		awsError(writer, errors.Errorf("number of calls exceeded %d", s.Config.ThrottlingThreshold), "TooManyRequestsException", http.StatusBadRequest)
		return
	}

	awsRequest, err := parseAWSHeaders(request)
	if err != nil {
		awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		return
	}

	defer s.pushCall()

	decoder := json.NewDecoder(request.Body)
	switch awsRequest.Action {
	case opDeleteTerminology:
		var input translate.DeleteTerminologyInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleDeleteTerminology(writer, input)
	case opDescribeTextTranslationJob:
		var input translate.DescribeTextTranslationJobInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleDescribeTextTranslationJob(writer, input)
	case opGetTerminology:
		var input translate.GetTerminologyInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleGetTerminology(writer, input)
	case opImportTerminology:
		var input translate.ImportTerminologyInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleImportTerminology(writer, input)
	case opListTerminologies:
		var input translate.ListTerminologiesInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleListTerminologies(writer, input)
	case opListTextTranslationJobs:
		var input translate.ListTextTranslationJobsInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleListTextTranslationJobs(writer, input)
	case opStartTextTranslationJob:
		var input translate.StartTextTranslationJobInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleStartTextTranslationJob(writer, input)
	case opStopTextTranslationJob:
		var input translate.StopTextTranslationJobInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandlerStopTextTranslationJob(writer, input)
	case opTranslateText:
		var input translate.TextInput
		if err := decoder.Decode(&input); err != nil {
			awsError(writer, err, "InvalidQueryParameter", http.StatusBadRequest)
		}
		s.HandleTranslateText(writer, input)
	default:
		awsError(writer, errors.Errorf("unrecognized action: %s", awsRequest.Action), "InvalidAction", http.StatusBadRequest)
	}
}

func (s *Server) UpdateJobs() {
	log.Debug("updating jobs")
	for _, job := range s.jobs {
		//TODO: Use the duration from config instead of now to update the jobs
		spew.Dump(job)
	}
}

func (s *Server) checkThrottling() bool {
	if s.Config.ThrottlingThreshold == 0 {
		return false
	}

	var (
		calls     int
		threshold = time.Now().Add(-1 * s.Config.ThrottlingPeriod)
	)
	for i := 0; i < s.calls.Len(); i++ {
		callTime := s.calls.Value.(time.Time)
		if callTime.After(threshold) {
			calls++
		}
	}

	if calls > s.Config.ThrottlingThreshold {
		return true
	}
	return false
}

func (s *Server) pushCall() {
	s.calls.Value = time.Now()
	s.calls = s.calls.Next()
}

////////////////////////////////////////////////////////////////////////////////

func (s *Server) HandleTranslateText(writer http.ResponseWriter, input translate.TextInput) {
	log.Infof("Action: TranslateText (%s->%s)", *input.SourceLanguageCode, *input.TargetLanguageCode)

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

func (s *Server) HandleStartTextTranslationJob(writer http.ResponseWriter, input translate.StartTextTranslationJobInput) {
	log.Info("Action: StartTextTranslationJob")

	now := time.Now()
	jobID := strconv.Itoa(len(s.jobs))
	job := translate.TextTranslationJobProperties{
		JobId:               aws.String(jobID),
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
			InputDocumentsCount:      aws.Int64(10),
			TranslatedDocumentsCount: aws.Int64(0),
		},
	}

	s.jobs[jobID] = &job
	log.Infof("job started: %s", spew.Sprint(job))

	writeOutput(writer, translate.StartTextTranslationJobOutput{
		JobId:     job.JobId,
		JobStatus: job.JobStatus,
	})
}

func (s *Server) HandleListTextTranslationJobs(writer http.ResponseWriter, input translate.ListTextTranslationJobsInput) {
	log.Infof("Action: ListTextTranslationJobs")

	jobs := []*translate.TextTranslationJobProperties{
		{
			DataAccessRoleArn: nil,
			EndTime:           &time.Time{},
			InputDataConfig: &translate.InputDataConfig{
				ContentType: nil,
				S3Uri:       nil,
			},
			JobDetails: &translate.JobDetails{
				DocumentsWithErrorsCount: nil,
				InputDocumentsCount:      nil,
				TranslatedDocumentsCount: nil,
			},
			JobId:     nil,
			JobName:   nil,
			JobStatus: nil,
			Message:   nil,
			OutputDataConfig: &translate.OutputDataConfig{
				S3Uri: nil,
			},
			SourceLanguageCode:  nil,
			SubmittedTime:       &time.Time{},
			TargetLanguageCodes: nil,
			TerminologyNames:    nil,
		},
	}
	output := translate.ListTextTranslationJobsOutput{
		NextToken:                        aws.String("test-token"),
		TextTranslationJobPropertiesList: jobs,
	}
	writeOutput(writer, output)
}

func (s *Server) HandleDescribeTextTranslationJob(writer http.ResponseWriter, input translate.DescribeTextTranslationJobInput) {
	log.Infof("Action: DescribeTextTranslationJob")
	job, ok := s.jobs[*input.JobId]
	if !ok {
		awsError(writer, errors.Errorf("job '%s' was not found", *input.JobId), "ResourceNotFoundException", http.StatusBadRequest)
		return
	}

	writeOutput(writer, *job)
}

func (s *Server) HandlerStopTextTranslationJob(writer http.ResponseWriter, input translate.StopTextTranslationJobInput) {
	log.Infof("Action: StopTextTranslationJob %s", *input.JobId)
	job, ok := s.jobs[*input.JobId]
	if !ok {
		awsError(writer, errors.Errorf("job '%s' was not found", *input.JobId), "ResourceNotFoundException", http.StatusBadRequest)
		return
	}
	job.JobStatus = aws.String(translate.JobStatusStopRequested)

	output := translate.StopTextTranslationJobOutput{
		JobId:     input.JobId,
		JobStatus: job.JobStatus,
	}
	writeOutput(writer, output)
}

func (s *Server) HandleImportTerminology(writer http.ResponseWriter, input translate.ImportTerminologyInput) {
	log.Infof("Action: ImportTerminology")

	output := translate.ImportTerminologyOutput{
		TerminologyProperties: &translate.TerminologyProperties{
			Arn:         nil,
			CreatedAt:   &time.Time{},
			Description: nil,
			EncryptionKey: &translate.EncryptionKey{
				Id:   nil,
				Type: nil,
			},
			LastUpdatedAt:       &time.Time{},
			Name:                nil,
			SizeBytes:           nil,
			SourceLanguageCode:  nil,
			TargetLanguageCodes: nil,
			TermCount:           nil,
		},
	}
	writeOutput(writer, output)
}

func (s *Server) HandleListTerminologies(writer http.ResponseWriter, input translate.ListTerminologiesInput) {
	log.Infof("Action: ListTerminologies")

	output := translate.ListTerminologiesOutput{
		NextToken:                 nil,
		TerminologyPropertiesList: nil,
	}
	writeOutput(writer, output)
}

func (s *Server) HandleGetTerminology(writer http.ResponseWriter, input translate.GetTerminologyInput) {
	log.Infof("Action: GetTerminology")

	output := translate.GetTerminologyOutput{
		TerminologyDataLocation: &translate.TerminologyDataLocation{
			Location:       nil,
			RepositoryType: nil,
		},
		TerminologyProperties: &translate.TerminologyProperties{
			Arn:         nil,
			CreatedAt:   &time.Time{},
			Description: nil,
			EncryptionKey: &translate.EncryptionKey{
				Id:   nil,
				Type: nil,
			},
			LastUpdatedAt:       &time.Time{},
			Name:                nil,
			SizeBytes:           nil,
			SourceLanguageCode:  nil,
			TargetLanguageCodes: nil,
			TermCount:           nil,
		},
	}
	writeOutput(writer, output)
}

func (s *Server) HandleDeleteTerminology(writer http.ResponseWriter, input translate.DeleteTerminologyInput) {
	log.Infof("Action: DeleteTerminology")

	output := translate.DeleteTerminologyOutput{}
	writeOutput(writer, output)
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
