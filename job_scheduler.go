package main

import (
	"github.com/apex/log"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/translate"
	"math"
	"math/rand"
	"strconv"
	"time"
)

//go:generate mockery -name=Scheduler -case=snake -inpkg -testonly

type Scheduler interface {
	Jobs() map[string]*translate.TextTranslationJobProperties
	AddJobStartRequest(job *translate.TextTranslationJobProperties, now time.Time) string
	AddJobStopRequest(job *translate.TextTranslationJobProperties, now time.Time)
	CheckStartRequest(job *translate.TextTranslationJobProperties, now time.Time) (startTime *time.Time, jobStarted bool)
	CheckStopRequest(job *translate.TextTranslationJobProperties, now time.Time) (stopTime *time.Time, jobStopped bool)
	CheckFailedJob(job *translate.TextTranslationJobProperties) bool
	StartJob(job *translate.TextTranslationJobProperties, startTime *time.Time)
	StopJob(job *translate.TextTranslationJobProperties, stopTime *time.Time)
	UpdateJob(job *translate.TextTranslationJobProperties, now time.Time)
}

type JobScheduler struct {
	Config *Config
	jobs   map[string]*translate.TextTranslationJobProperties

	jobsStartTime map[string]time.Time
	startRequests map[string]time.Time
	stopRequests  map[string]time.Time
	markedToFail  map[string]bool
}

func NewJobScheduler(config *Config, jobs ...translate.TextTranslationJobProperties) *JobScheduler {
	s := JobScheduler{
		Config: config,

		jobsStartTime: make(map[string]time.Time),
		startRequests: make(map[string]time.Time),
		stopRequests:  make(map[string]time.Time),
		markedToFail:  make(map[string]bool),
	}

	now := time.Now()
	for _, job := range jobs {
		jobID := *job.JobId
		s.jobs[jobID] = &job

		switch *job.JobStatus {
		case translate.JobStatusSubmitted:
			s.startRequests[jobID] = now
		case translate.JobStatusStopRequested:
			s.stopRequests[jobID] = now
		case translate.JobStatusInProgress:
			if job.SubmittedTime != nil {
				jobStartTime := job.SubmittedTime.Add(s.Config.StartDelay)
				s.StartJob(&job, &jobStartTime)
			}
			s.StartJob(&job, &now)
		}
	}

	return &s
}

func UpdateScheduler(s Scheduler) {
	log.Debug("updating jobs")
	now := time.Now()
	for jobID, job := range s.Jobs() {
		if *job.JobStatus == translate.JobStatusSubmitted {
			if startTime, ok := s.CheckStartRequest(job, now); ok {
				s.StartJob(job, startTime)
			}
		}

		if *job.JobStatus == translate.JobStatusStopRequested {
			if stopTime, ok := s.CheckStopRequest(job, now); ok {
				s.StopJob(job, stopTime)
			}
		}

		if *job.JobStatus == translate.JobStatusInProgress {
			log.Debugf("%s: checking running job", jobID)

			if s.CheckFailedJob(job) {
				//TODO: fail job
			}

			s.UpdateJob(job, now)
		}
	}
}

func (s *JobScheduler) Jobs() map[string]*translate.TextTranslationJobProperties {
	return s.jobs
}

func (s *JobScheduler) AddJobStartRequest(job *translate.TextTranslationJobProperties, now time.Time) string {
	jobID := strconv.Itoa(len(s.jobs))
	job.JobId = &jobID
	s.jobs[jobID] = job
	s.startRequests[jobID] = now

	return jobID
}

func (s *JobScheduler) AddJobStopRequest(job *translate.TextTranslationJobProperties, now time.Time) {
	jobID := *job.JobId
	delete(s.startRequests, jobID)
	s.stopRequests[jobID] = now
}

func (s *JobScheduler) CheckStartRequest(job *translate.TextTranslationJobProperties, now time.Time) (startTime *time.Time, jobStarted bool) {
	jobID := *job.JobId
	log.Debugf("%s: checking start request", jobID)

	submittedTime, ok := s.startRequests[jobID]
	if !ok {
		return nil, false
	}

	jobStartTime := submittedTime.Add(s.Config.StartDelay)
	if jobStartTime.After(now) {
		return &jobStartTime, true
	}

	log.Debugf("%s: start time %s not reached yet", jobStartTime.Format(time.RFC3339))
	return nil, false
}

func (s *JobScheduler) CheckStopRequest(job *translate.TextTranslationJobProperties, now time.Time) (stopTime *time.Time, jobStopped bool) {
	jobID := *job.JobId
	log.Debugf("%s: checking stop request", jobID)

	submittedTime, ok := s.stopRequests[jobID]
	if !ok {
		return nil, false
	}

	jobStopTime := submittedTime.Add(s.Config.StopDelay)
	if jobStopTime.After(now) {
		return &jobStopTime, true
	}

	log.Debugf("%s: stop time %s not reached yet", jobStopTime.Format(time.RFC3339))
	return nil, false
}

func (s *JobScheduler) CheckFailedJob(_ *translate.TextTranslationJobProperties) bool {
	return weightedChoice(s.Config.FailRate)
}

func (s *JobScheduler) StartJob(job *translate.TextTranslationJobProperties, startTime *time.Time) {
	jobID := *job.JobId
	log.Debugf("%s: starting job", jobID)

	//TODO: Check number of input files in job worker

	job.JobStatus = aws.String(translate.JobStatusInProgress)
	delete(s.startRequests, jobID)
	s.jobsStartTime[jobID] = *startTime

	log.Infof("%s: changed state to %s", jobID, translate.JobStatusInProgress)
}

func (s *JobScheduler) StopJob(job *translate.TextTranslationJobProperties, stopTime *time.Time) {
	jobID := *job.JobId
	log.Debugf("%s: stopping job", jobID)

	job.JobStatus = aws.String(translate.JobStatusStopped)
	job.EndTime = stopTime
	delete(s.stopRequests, jobID)

	log.Infof("%s: changed state %s", jobID, translate.JobStatusStopped)
}

func (s *JobScheduler) UpdateJob(job *translate.TextTranslationJobProperties, now time.Time) {
	jobID := *job.JobId
	log.Debugf("%s: updating running job", jobID)

	startTime := s.jobsStartTime[jobID]

	jobDuration := normalDuration(s.Config.JobDuration, s.Config.JobDurationDeviation)
	endTime := startTime.Add(jobDuration)

	inputDocumentCount := *job.JobDetails.InputDocumentsCount
	translatedDocumentsCount := *job.JobDetails.TranslatedDocumentsCount
	documentsWithErrorsCount := *job.JobDetails.DocumentsWithErrorsCount

	if now.After(endTime) {
		log.Debugf("%s: job completed", jobID)

		processedDocuments := translatedDocumentsCount + documentsWithErrorsCount
		if processedDocuments < inputDocumentCount {
			job.JobDetails.TranslatedDocumentsCount = aws.Int64(inputDocumentCount - documentsWithErrorsCount)
		}

		job.JobStatus = aws.String(translate.JobStatusCompleted)
		job.EndTime = &endTime
		delete(s.stopRequests, jobID)
		return
	}

	// TODO: move to job worker
	jobRunningTime := now.Sub(startTime)
	completionRatio := float64(jobRunningTime.Milliseconds()) / float64(jobDuration.Milliseconds())
	completedFiles := math.Floor(float64(inputDocumentCount) * completionRatio)

	if count := int64(completedFiles * (1 - s.Config.ErrorRate)); count > translatedDocumentsCount {
		translatedDocumentsCount = count
	}
	if count := int64(completedFiles * s.Config.ErrorRate); count > documentsWithErrorsCount {
		documentsWithErrorsCount = count
	}
	if translatedDocumentsCount+documentsWithErrorsCount > inputDocumentCount {
		translatedDocumentsCount = inputDocumentCount - documentsWithErrorsCount
	}

	job.JobDetails.TranslatedDocumentsCount = aws.Int64(translatedDocumentsCount)
	job.JobDetails.DocumentsWithErrorsCount = aws.Int64(documentsWithErrorsCount)
}

////////////////////////////////////////////////////////////////////////////////

func normalDuration(mean time.Duration, deviation time.Duration) time.Duration {
	var (
		mu    = float64(mean.Milliseconds())
		sigma = float64(deviation.Milliseconds())
	)
	x := rand.NormFloat64()*sigma + mu
	return time.Duration(int64(math.Round(x))) * time.Millisecond
}

func weightedChoice(probabilityPercent int) bool {
	r := rand.Intn(99) + 1
	return r <= probabilityPercent
}
