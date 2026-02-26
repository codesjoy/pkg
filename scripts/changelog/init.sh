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

source "$(dirname "${BASH_SOURCE[0]}")/../lib/init.sh"
set -o nounset

CHANGELOG_CONFIG="${CHANGELOG_CONFIG:-${ROOT_DIR}/.chglog/config.yml}"
CHANGELOG_TEMPLATE="${CHANGELOG_TEMPLATE:-${ROOT_DIR}/.chglog/CHANGELOG.tpl.md}"
CHANGELOG_STATE_FILE="${CHANGELOG_STATE_FILE:-${ROOT_DIR}/.chglog/state.env}"
CHANGELOG_ARCHIVE_DIR="${CHANGELOG_ARCHIVE_DIR:-${ROOT_DIR}/.chglog/archive}"

STATE_DIR="$(dirname "${CHANGELOG_STATE_FILE}")"
STATE_EXAMPLE_FILE="${STATE_DIR}/state.env.example"
ARCHIVE_GITKEEP_FILE="${CHANGELOG_ARCHIVE_DIR}/.gitkeep"

ensure_dir() {
	local dir_path="${1}"
	if [[ -d "${dir_path}" ]]; then
		log::info "Directory exists, skipping: ${dir_path}"
		return 0
	fi

	mkdir -p "${dir_path}"
	log::success "Created directory: ${dir_path}"
}

write_if_missing() {
	local file_path="${1}"
	local content_renderer="${2}"

	if [[ -f "${file_path}" ]]; then
		log::warn "File exists, skipping: ${file_path}"
		return 0
	fi

	mkdir -p "$(dirname "${file_path}")"
	"${content_renderer}" >"${file_path}"
	log::success "Created file: ${file_path}"
}

render_default_config() {
	cat <<'EOF'
style: github
template: .chglog/CHANGELOG.tpl.md
info:
  title: CHANGELOG
  repository_url: https://github.com/codesjoy/pkg
options:
  commits:
    filters:
      Type:
        - feat
        - fix
        - docs
        - style
        - refactor
        - perf
        - test
        - build
        - ci
        - chore
        - revert
  commit_groups:
    title_maps:
      feat: Features
      fix: Bug Fixes
      docs: Documentation
      style: Styles
      refactor: Refactors
      perf: Performance Improvements
      test: Tests
      build: Build System
      ci: CI
      chore: Chores
      revert: Reverts
  header:
    pattern: "^([[:alnum:]]+)(?:\\(([[:alnum:]_./\\-\\s]+)\\))?(!)?:\\s(.+)$"
    pattern_maps:
      - Type
      - Scope
      - Breaking
      - Subject
  notes:
    keywords:
      - BREAKING CHANGE
EOF
}

render_default_template() {
	cat <<'EOF'
{{- range .Versions }}
<a name="{{ .Tag.Name }}"></a>
## {{- if .Tag.Previous }} [{{ .Tag.Name }}]({{ $.Info.RepositoryURL }}/compare/{{ .Tag.Previous.Name }}...{{ .Tag.Name }}){{- else }} {{ .Tag.Name }}{{- end }} ({{ datetime "2006-01-02" .Tag.Date }})

{{- range .CommitGroups }}
### {{ .Title }}

{{- range .Commits }}
- {{- if .Scope }} **{{ .Scope }}:** {{- end }} {{ .Subject }}
{{- end }}
{{- end }}

{{- if .RevertCommits }}
### Reverts

{{- range .RevertCommits }}
- {{ .Revert.Header }}
{{- end }}
{{- end }}

{{- if .NoteGroups }}
{{- range .NoteGroups }}
### {{ .Title }}

{{- range .Notes }}
{{ .Body }}
{{- end }}
{{- end }}
{{- end }}

{{- end }}
EOF
}

render_state_example() {
	cat <<'EOF'
# Changelog state file example.
# Copy to .chglog/state.env if you want to bootstrap state manually.
#
# BASE_SHA: baseline commit for managed incremental range (BASE_SHA..HEAD).
# LAST_SHA: last generated HEAD commit.
# CURRENT_BUCKET: current archive bucket label (YYYY-MM / YYYY-Www / none).
BASE_SHA=
LAST_SHA=
CURRENT_BUCKET=
EOF
}

render_empty_file() {
	printf ''
}

main() {
	log::info "Initializing changelog scaffold"
	ensure_dir "$(dirname "${CHANGELOG_CONFIG}")"
	ensure_dir "$(dirname "${CHANGELOG_TEMPLATE}")"
	ensure_dir "${STATE_DIR}"
	ensure_dir "${CHANGELOG_ARCHIVE_DIR}"

	write_if_missing "${CHANGELOG_CONFIG}" render_default_config
	write_if_missing "${CHANGELOG_TEMPLATE}" render_default_template
	write_if_missing "${STATE_EXAMPLE_FILE}" render_state_example
	write_if_missing "${ARCHIVE_GITKEEP_FILE}" render_empty_file

	log::success "Changelog scaffold initialization complete"
}

main "$@"
