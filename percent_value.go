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
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"strconv"
	"strings"
)

const typeName = "percent"

type PercentValue struct {
	Value int
}

func (p PercentValue) String() string {
	return fmt.Sprintf("%d%%", p.Value)
}

func (p PercentValue) Set(s string) error {
	s = strings.TrimRight(s, "%")
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	if v < 0 {
		return errors.New("a percentage value cannot be lower than 0%")
	}
	if v > 100 {
		return errors.New("a percentage value cannot be higher than 100%")
	}
	p.Value = v
	return nil
}

func (p PercentValue) Type() string {
	return typeName
}

func GetPercentFlag(flags *pflag.FlagSet, name string) (int, error) {
	f := flags.Lookup(name)
	if f == nil {
		err := fmt.Errorf("flag accessed but not defined: %s", name)
		return 0, err
	}

	if f.Value.Type() != typeName {
		err := fmt.Errorf("trying to get %s value of flag of type %s", typeName, f.Value.Type())
		return 0, err
	}

	v := PercentValue{}
	if err := v.Set(f.Value.String()); err != nil {
		return 0, err
	}

	return v.Value, nil
}
