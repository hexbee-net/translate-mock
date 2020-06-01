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
	"github.com/apex/log/handlers/text"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/translate"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
	"time"
)

const Version = "0.0.1"

const (
	flagPort                 = "port"
	flagDebug                = "debug"
	flagJobDuration          = "job-duration"
	flagJobDurationDeviation = "job-deviation"
	flagStartDelay           = "start-delay"
	flagStopDelay            = "stop-delay"
	flagThrottlingThreshold  = "throttling-threshold"
	flagThrottlingPeriod     = "throttling-period"
	flagErrorRate            = "error-rate"
	flagFailRate             = "faile-rate"
)

func main() {
	log.SetHandler(text.New(os.Stdout))
	log.SetLevel(log.DebugLevel)

	root := &cobra.Command{
		Use:     "translate-mock",
		Short:   "AWS Translate local mock server",
		Version: Version,
		Run:     startServer,
	}
	root.Flags().IntP(flagPort, "p", 4670, "Port for the http server to listen on.")
	root.Flags().Bool(flagDebug, false, "Display debug messages")

	root.Flags().DurationP(flagJobDuration, "d", 10*time.Second, "Time for a job to reach completion one it is started.")
	root.Flags().DurationP(flagJobDurationDeviation, "v", 0, "Amount by which the time to completion is allowed to change.")
	root.Flags().DurationP(flagStartDelay, "a", 1*time.Second, "Delay between when a job is submitted and when it actually starts.")
	root.Flags().DurationP(flagStopDelay, "o", 1*time.Second, "Delay between when a job stop is requested and when the job actually stops.")
	root.Flags().IntP(flagThrottlingThreshold, "r", 0, "Max number of calls authorized during the throttling period. 0 means not throttling.")
	root.Flags().DurationP(flagThrottlingPeriod, "t", 0, "Duration of the throttling period. If the number of calls exceeds throttling-threshold during this period, the call will fail.")
	root.Flags().VarP(PercentValue{}, flagErrorRate, "e", "Percentage of documents with errors on completion for each job.")
	root.Flags().VarP(PercentValue{}, flagFailRate, "f", "Probability of jobs failure in each update cycle.")

	root.AddCommand(&cobra.Command{
		Use:   "generate-jobs-skeleton",
		Short: "Generate the json skeleton of the initial jobs JSON input file",
		Run:   generateJobsSkeleton,
	})

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func startServer(cmd *cobra.Command, _ []string) {
	debug, err := cmd.Flags().GetBool(flagDebug)
	if err != nil {
		panic(err)
	}
	if debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Infof("starting...")
	port, err := cmd.Flags().GetInt(flagPort)
	if err != nil {
		panic(err)
	}

	config := loadConfig(cmd.Flags())
	log.Debugf("configuration: %s", spew.Sprint(config))

	server := NewServer(config)
	server.Run(port)
}

func loadConfig(flags *pflag.FlagSet) Config {
	mustDuration := func(name string) time.Duration {
		v, err := flags.GetDuration(name)
		if err != nil {
			panic(err)
		}
		return v
	}
	mustInt := func(name string) int {
		v, err := flags.GetInt(name)
		if err != nil {
			panic(err)
		}
		return v
	}
	mustPercent := func(name string) int {
		v, err := GetPercentFlag(flags, name)
		if err != nil {
			panic(err)
		}
		return v
	}
	config := Config{
		JobDuration:          mustDuration(flagJobDuration),
		JobDurationDeviation: mustDuration(flagJobDurationDeviation),
		StartDelay:           mustDuration(flagStartDelay),
		StopDelay:            mustDuration(flagStopDelay),
		ThrottlingThreshold:  mustInt(flagThrottlingThreshold),
		ThrottlingPeriod:     mustDuration(flagThrottlingPeriod),
		ErrorRate:            float64(mustPercent(flagErrorRate)) / 100,
		FailRate:             mustPercent(flagFailRate),
	}
	return config
}

func generateJobsSkeleton(_ *cobra.Command, _ []string) {
	template := []translate.TextTranslationJobProperties{{
		JobId:               aws.String("1234"),
		JobName:             aws.String("test-job"),
		DataAccessRoleArn:   aws.String("arn:aws:iam::123456789012:role/test-role"),
		EndTime:             aws.Time(time.Date(2015, 10, 21, 16, 29, 0, 0, time.UTC)),
		JobStatus:           aws.String(translate.JobStatusSubmitted),
		Message:             aws.String(""),
		SourceLanguageCode:  aws.String("en"),
		TargetLanguageCodes: aws.StringSlice([]string{"fr", "de"}),
		SubmittedTime:       &time.Time{},
		TerminologyNames:    aws.StringSlice([]string{"terminology-1", "terminology-2"}),
		InputDataConfig: &translate.InputDataConfig{
			ContentType: aws.String("text/html"),
			S3Uri:       aws.String("s3://input-bucket/input-key"),
		},
		OutputDataConfig: &translate.OutputDataConfig{
			S3Uri: aws.String("s3://output-bucket/output-key"),
		},
		JobDetails: &translate.JobDetails{
			DocumentsWithErrorsCount: aws.Int64(5),
			InputDocumentsCount:      aws.Int64(20),
			TranslatedDocumentsCount: aws.Int64(10),
		},
	}}
	jsonTemplate, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(jsonTemplate))
}
