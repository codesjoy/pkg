#!/usr/bin/env bash
# Copyright 2022 The codesjoy Authors.
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


set -o errexit
set -o nounset
set -o pipefail

source "$(dirname "${BASH_SOURCE[0]}")/lib/init.sh"

if [ ! -f "${ROOT_DIR}/go.work" ]; then
  log::info "Initializing go.work..."
  cd "${ROOT_DIR}"
  go work init

  # Auto-discover and add all modules
  for go_mod in $(find "${ROOT_DIR}" -name "go.mod" -type f | sort); do
    module_dir=$(dirname "${go_mod}")
    relative_dir=${module_dir#"${ROOT_DIR}/"}
    log::info "Adding ${relative_dir} to workspace"
    go work use "${relative_dir}"
  done

  log::success "Go workspace initialized with $(find "${ROOT_DIR}" -name "go.mod" -type f | wc -l) modules"
else
  log::info "go.work already exists"
fi