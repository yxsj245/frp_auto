// Copyright 2025 The frp Authors
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

package model

type AssistancePortInfo struct {
	LocalPort  int    `json:"localPort"`
	Type       string `json:"type"`
	RemotePort int    `json:"remotePort,omitempty"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
}

type AssistanceInfoResp struct {
	Code       string               `json:"code"`
	User       string               `json:"user"`
	Remark     string               `json:"remark"`
	Status     string               `json:"status"`
	Ports      []AssistancePortInfo `json:"ports"`
	CreatedAt  int64                `json:"createdAt"`
	ApprovedAt int64                `json:"approvedAt,omitempty"`
	ClosedAt   int64                `json:"closedAt,omitempty"`
}
