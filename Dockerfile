# Copyright 2017, 2019, 2020 the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.24-openshift-4.20 AS build
WORKDIR /go/src/github.com/openshift/hypershift-oadp-plugin
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/hypershift-oadp-plugin .

FROM registry.access.redhat.com/ubi9-minimal
RUN mkdir /plugins
COPY --from=build /go/bin/hypershift-oadp-plugin /plugins/
USER 65532:65532
ENTRYPOINT ["/bin/bash", "-c", "cp /plugins/* /target/."]
