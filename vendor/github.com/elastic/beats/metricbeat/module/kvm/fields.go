// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Code generated by beats/dev-tools/cmd/asset/asset.go - DO NOT EDIT.

package kvm

import (
	"github.com/elastic/beats/libbeat/asset"
)

func init() {
	if err := asset.SetFields("metricbeat", "kvm", asset.ModuleFieldsPri, AssetKvm); err != nil {
		panic(err)
	}
}

// AssetKvm returns asset data.
// This is the base64 encoded gzipped contents of ../metricbeat/module/kvm.
func AssetKvm() string {
	return "eJyskDFuwzAMRXed4iN7LqChU9ceQq3YQLBoGrLsQrcvnEZFotByh3Lw4G+//8gzBioWw8oGyCFHsjgNK58MkCiSm8ninbIzgKf5I4UpBxktXgyA7T+w+CWSAT4DRT/ba3DG6JgqeJtcJrK4JFmm2xuF9wi5B3lhJp6zy7+Rxtzl3iKN0u5Zp1V5FGogPaFDrZ95I5ZUdLDucu+zPZW4Og1UviR59YtDs8Zur6uqrC4uPZco4+V/RLSmahGel+3UH1a/CrswPlO79+/f/q+dV/R3AAAA///hct15"
}
