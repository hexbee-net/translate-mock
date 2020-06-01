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
	"time"
)

type Config struct {
	JobDuration          time.Duration // Time for a job to reach completion one it is started.
	JobDurationDeviation time.Duration // Standard deviation of the job completion time.
	StartDelay           time.Duration // Delay between when a job is submitted and when it actually starts.
	StopDelay            time.Duration // Delay between when a job stop is requested and when the job actually stops.
	ThrottlingThreshold  int           // Max number of calls authorized during the throttling period. 0 means not throttling.
	ThrottlingPeriod     time.Duration // Duration of the throttling period. If the number of calls exceeds throttling-threshold during this period, the call will fail.
	ErrorRate            float64       // Percentage of documents with errors on completion for each job.
	FailRate             int           // Probability of jobs failure in each update cycle.
}
