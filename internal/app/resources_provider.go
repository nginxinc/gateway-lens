/*
Copyright 2026 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"github.com/nginxinc/gateway-lens/internal/topology"
)

// liveResourcesReader provides read access to the latest synchronized Gateway API resources.
type liveResourcesReader interface {
	// Get returns a snapshot of the current Gateway API resources.
	Get() topology.GatewayAPIResources
	// Subscribe returns a channel that receives a notification each time the resource store changes.
	Subscribe() <-chan struct{}
	// Unsubscribe removes a previously subscribed channel and closes it.
	Unsubscribe(ch <-chan struct{})
}
