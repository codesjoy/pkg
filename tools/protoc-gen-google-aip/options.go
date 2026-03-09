// Copyright 2022 The codesjoy Authors.
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
	"flag"
	"fmt"
	"strings"
)

const defaultFeatures = "resources,field_behavior,method_signature"

type config struct {
	features string
}

type featureSet struct {
	resources       bool
	fieldBehavior   bool
	methodSignature bool
}

func parseConfig(fs *flag.FlagSet) (config, error) {
	cfg := config{}
	fs.StringVar(
		&cfg.features,
		"features",
		defaultFeatures,
		"comma-separated features: resources,field_behavior,method_signature",
	)
	return cfg, nil
}

func parseFeatureSet(raw string) (featureSet, error) {
	var out featureSet
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}

	for _, token := range strings.Split(raw, ",") {
		feature := normalizeFeatureName(token)
		switch feature {
		case "":
			continue
		case "resources":
			out.resources = true
		case "field_behavior":
			out.fieldBehavior = true
		case "method_signature":
			out.methodSignature = true
		default:
			return featureSet{}, fmt.Errorf("unknown feature %q", token)
		}
	}

	return out, nil
}

func normalizeFeatureName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "_")
	return value
}
