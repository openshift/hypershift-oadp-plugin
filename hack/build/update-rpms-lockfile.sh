#!/usr/bin/env bash
#
# update-rpms-lockfile.sh - Regenerates rpms.lock.yaml using rpm-lockfile-prototype
#
# This script runs rpm-lockfile-prototype inside a UBI9 container to resolve
# the latest RPM versions and update the lockfile used by Konflux/cachi2.
#
# Requirements:
#   - podman (or docker)
#   - For RHEL CDN repos: RHEL subscription certs at /etc/pki/entitlement/
#   - Without subscription: uses UBI repos and rewrites URLs to RHEL CDN format
#
# Usage:
#   ./hack/build/update-rpms-lockfile.sh
#   CONTAINER_ENGINE=docker ./hack/build/update-rpms-lockfile.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RPMS_IN="${SCRIPT_DIR}/rpms.in.yaml"
RPMS_LOCK="${SCRIPT_DIR}/rpms.lock.yaml"
REDHAT_REPO="${SCRIPT_DIR}/redhat.repo"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-}"
UBI_IMAGE="${UBI_IMAGE:-registry.access.redhat.com/ubi9:latest}"
RPM_LOCKFILE_REPO="${RPM_LOCKFILE_REPO:-https://github.com/konflux-ci/rpm-lockfile-prototype.git}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# Detect container engine
detect_container_engine() {
    if [[ -n "${CONTAINER_ENGINE}" ]]; then
        if command -v "${CONTAINER_ENGINE}" &>/dev/null; then
            return 0
        fi
        error "Specified CONTAINER_ENGINE '${CONTAINER_ENGINE}' not found"
        return 1
    fi

    if command -v podman &>/dev/null; then
        CONTAINER_ENGINE="podman"
    elif command -v docker &>/dev/null; then
        CONTAINER_ENGINE="docker"
    else
        error "No container engine found. Install podman or docker."
        return 1
    fi
    info "Using container engine: ${CONTAINER_ENGINE}"
}

# Check if RHEL subscription certs are available
has_rhel_subscription() {
    local entitlement_dir="/etc/pki/entitlement"
    if [[ -d "${entitlement_dir}" ]] && ls "${entitlement_dir}"/*.pem &>/dev/null; then
        return 0
    fi
    return 1
}

# Build volume mount args for subscription certs
subscription_mounts() {
    local mounts=""
    if has_rhel_subscription; then
        mounts="-v /etc/pki/entitlement:/etc/pki/entitlement:ro"
        if [[ -d /etc/rhsm ]]; then
            mounts="${mounts} -v /etc/rhsm:/etc/rhsm:ro"
        fi
        if [[ -d /etc/pki/rpm-gpg ]]; then
            mounts="${mounts} -v /etc/pki/rpm-gpg:/etc/pki/rpm-gpg:ro"
        fi
    fi
    echo "${mounts}"
}

# Generate a UBI-compatible repo file derived from the existing redhat.repo
# Transforms RHEL CDN URLs to public UBI CDN equivalents and strips
# subscription-dependent settings (SSL certs, GPG checks).
# Repos without UBI equivalents (e.g. rhocp-*) are skipped.
create_ubi_repo_from_redhat() {
    local redhat_repo="$1"
    python3 - "${redhat_repo}" <<'PYEOF'
import sys
import re

RHEL_CDN = "https://cdn.redhat.com/content/dist/rhel9/"

repo_file = sys.argv[1]
with open(repo_file) as f:
    content = f.read()

# Parse repo sections
sections = re.split(r'(?=^\[)', content, flags=re.MULTILINE)

for section in sections:
    section = section.strip()
    if not section:
        continue

    # Extract repo id
    header_match = re.match(r'^\[(.+)\]', section)
    if not header_match:
        continue
    repo_id = header_match.group(1)

    # Parse key-value pairs
    kvs = {}
    for line in section.split('\n')[1:]:
        line = line.strip()
        if '=' in line:
            key, val = line.split('=', 1)
            kvs[key.strip()] = val.strip()

    baseurl = kvs.get('baseurl', '')

    # Only transform repos whose baseurl points to the RHEL CDN baseos/appstream
    if RHEL_CDN not in baseurl:
        # Skip repos without UBI equivalent (e.g. rhocp, layered products)
        continue

    # Transform repo ID: rhel-9-for-$basearch-X -> ubi-9-X
    ubi_repo_id = re.sub(r'^rhel-9-for-\$basearch-', 'ubi-9-', repo_id)

    # Transform baseurl: RHEL CDN -> UBI CDN, replace $releasever with 9
    ubi_baseurl = baseurl.replace(
        'cdn.redhat.com/content/dist/rhel9/',
        'cdn-ubi.redhat.com/content/public/ubi/dist/ubi9/'
    ).replace('$releasever/', '9/')

    # Transform name
    name = kvs.get('name', ubi_repo_id)
    ubi_name = name.replace('Red Hat Enterprise Linux 9', 'Red Hat UBI 9')

    print(f'[{ubi_repo_id}]')
    print(f'baseurl = {ubi_baseurl}')
    print(f'enabled = {kvs.get("enabled", "1")}')
    print('gpgcheck = 0')
    print(f'name = {ubi_name}')
    print()
PYEOF
}

# Create a modified rpms.in.yaml that merges reinstallPackages into packages
# so that --bare mode (no base system) can resolve all packages.
create_bare_compatible_rpms_in() {
    local rpms_in="$1"
    local repo_ref="$2"
    python3 - "${rpms_in}" "${repo_ref}" <<'PYEOF'
import sys
import re

rpms_in_path = sys.argv[1]
repo_ref = sys.argv[2]

with open(rpms_in_path) as f:
    content = f.read()

# Extract packages list
packages = re.findall(r'packages:\s*\n((?:\s+-\s+\S+\n?)+)', content)
pkg_list = []
if packages:
    pkg_list = [p.strip().lstrip('- ') for p in packages[0].strip().split('\n')]

# Extract reinstallPackages and merge them into packages
reinstall = re.findall(r'reinstallPackages:\s*\n((?:\s+-\s+\S+\n?)+)', content)
if reinstall:
    reinstall_list = [p.strip().lstrip('- ') for p in reinstall[0].strip().split('\n')]
    for pkg in reinstall_list:
        if pkg not in pkg_list:
            pkg_list.append(pkg)

# Extract arches
arches = re.findall(r'arches:\s*\n((?:\s+-\s+\S+\n?)+)', content)
arch_list = []
if arches:
    arch_list = [a.strip().lstrip('- ') for a in arches[0].strip().split('\n')]

# Output modified rpms.in.yaml
print("packages:")
for pkg in pkg_list:
    print(f"  - {pkg}")
print("arches:")
for arch in arch_list:
    print(f"  - {arch}")
print("contentOrigin:")
print("  repofiles:")
print(f"  - {repo_ref}")
PYEOF
}

# Transform the generated lockfile from UBI format to RHEL CDN format and
# filter to only keep the packages explicitly listed in rpms.in.yaml.
# This is needed because --bare resolves all transitive dependencies,
# but the lockfile should only contain the directly requested packages.
transform_and_filter_lockfile() {
    local lockfile="$1"
    local rpms_in="$2"

    python3 - "${lockfile}" "${rpms_in}" <<'PYEOF'
import sys
import re

lockfile_path = sys.argv[1]
rpms_in_path = sys.argv[2]

# Parse rpms.in.yaml to get the list of wanted packages
with open(rpms_in_path) as f:
    in_content = f.read()

wanted_pkgs = set()
for match in re.findall(r'packages:\s*\n((?:\s+-\s+\S+\n?)+)', in_content):
    for line in match.strip().split('\n'):
        wanted_pkgs.add(line.strip().lstrip('- '))
for match in re.findall(r'reinstallPackages:\s*\n((?:\s+-\s+\S+\n?)+)', in_content):
    for line in match.strip().split('\n'):
        wanted_pkgs.add(line.strip().lstrip('- '))

# Parse the generated lockfile line by line (avoiding pyyaml dependency)
with open(lockfile_path) as f:
    lines = f.readlines()

# Build a structured representation by tracking indentation and context
current_arch = None
arches = []  # list of {arch, packages, source}
current_section = None  # 'packages' or 'source'
current_entry = {}
current_entry_lines = []

def flush_entry():
    global current_entry, current_entry_lines, current_arch, current_section
    if current_entry and current_arch is not None:
        name = current_entry.get('name', '')
        if name in wanted_pkgs:
            arch_data = arches[-1] if arches else None
            if arch_data and current_section:
                arch_data[current_section].append(current_entry.copy())
    current_entry = {}
    current_entry_lines = []

for line in lines:
    stripped = line.strip()

    # Detect arch block
    arch_match = re.match(r'^-\s*arch:\s*(\S+)', stripped)
    if arch_match:
        flush_entry()
        current_arch = arch_match.group(1)
        arches.append({'arch': current_arch, 'packages': [], 'source': []})
        current_section = None
        continue

    # Detect section (packages or source)
    if stripped == 'packages:':
        flush_entry()
        current_section = 'packages'
        continue
    if stripped == 'source:':
        flush_entry()
        current_section = 'source'
        continue

    # Skip non-package lines
    if current_section is None:
        continue

    # Detect new entry (starts with "- ")
    entry_start = re.match(r'^-\s+(\w+):\s*(.*)', stripped)
    if entry_start:
        flush_entry()
        key = entry_start.group(1)
        val = entry_start.group(2)
        current_entry[key] = val
        continue

    # Continuation of entry
    kv_match = re.match(r'^(\w+):\s*(.*)', stripped)
    if kv_match and current_section:
        key = kv_match.group(1)
        val = kv_match.group(2)
        current_entry[key] = val

flush_entry()

# Now output the filtered lockfile in the expected format
# Transform UBI references to RHEL CDN
def transform_repoid(repoid, arch):
    if repoid.startswith('ubi-9-'):
        suffix = repoid[len('ubi-9-'):]
        return f'rhel-9-for-{arch}-{suffix}'
    return repoid

def transform_url(url):
    return url.replace(
        'cdn-ubi.redhat.com/content/public/ubi/dist/ubi9',
        'cdn.redhat.com/content/dist/rhel9'
    )

output_lines = ['arches:\n']
for arch_data in arches:
    arch = arch_data['arch']
    output_lines.append(f'- arch: {arch}\n')
    output_lines.append('  module_metadata: []\n')

    output_lines.append('  packages:\n')
    for pkg in arch_data['packages']:
        output_lines.append(f'  - checksum: {pkg.get("checksum", "")}\n')
        output_lines.append(f'    evr: {pkg.get("evr", "")}\n')
        output_lines.append(f'    name: {pkg.get("name", "")}\n')
        output_lines.append(f'    repoid: {transform_repoid(pkg.get("repoid", ""), arch)}\n')
        output_lines.append(f'    size: {pkg.get("size", "")}\n')
        output_lines.append(f'    sourcerpm: {pkg.get("sourcerpm", "")}\n')
        output_lines.append(f'    url: {transform_url(pkg.get("url", ""))}\n')

    output_lines.append('  source:\n')
    for pkg in arch_data['source']:
        output_lines.append(f'  - checksum: {pkg.get("checksum", "")}\n')
        output_lines.append(f'    evr: {pkg.get("evr", "")}\n')
        output_lines.append(f'    name: {pkg.get("name", "")}\n')
        output_lines.append(f'    repoid: {transform_repoid(pkg.get("repoid", ""), arch)}\n')
        output_lines.append(f'    size: {pkg.get("size", "")}\n')
        output_lines.append(f'    url: {transform_url(pkg.get("url", ""))}\n')

output_lines.append('lockfileVendor: redhat\n')
output_lines.append('lockfileVersion: 1\n')

with open(lockfile_path, 'w') as f:
    f.writelines(output_lines)
PYEOF
}

# Run rpm-lockfile-prototype in a container
run_lockfile_generator() {
    local use_ubi_repos=false
    local extra_mounts=""
    local tmpdir=""
    local container_exit=0

    tmpdir="$(mktemp -d)"
    trap 'rm -rf "${tmpdir}"' EXIT

    if has_rhel_subscription; then
        info "RHEL subscription certs found, using RHEL CDN repos"
        extra_mounts="$(subscription_mounts)"

        # shellcheck disable=SC2086
        ${CONTAINER_ENGINE} run --rm \
            -v "${SCRIPT_DIR}:/work:Z" \
            ${extra_mounts} \
            ${UBI_IMAGE} bash -c "
                dnf install -y python3-pip git &>/dev/null
                pip install --quiet git+${RPM_LOCKFILE_REPO} 2>/dev/null
                rpm-lockfile-prototype --bare --outfile=/work/rpms.lock.yaml /work/rpms.in.yaml
            " || container_exit=$?
    else
        warn "No RHEL subscription certs found at /etc/pki/entitlement/"
        warn "Using UBI repos (public) with URL transformation to RHEL CDN format"
        use_ubi_repos=true

        # Generate UBI repo from the existing redhat.repo
        create_ubi_repo_from_redhat "${REDHAT_REPO}" > "${tmpdir}/ubi.repo"

        # Create bare-compatible rpms.in.yaml (merge reinstallPackages into packages)
        create_bare_compatible_rpms_in "${RPMS_IN}" "./ubi.repo" > "${tmpdir}/rpms.in.yaml"

        info "Installing rpm-lockfile-prototype and resolving packages..."

        # shellcheck disable=SC2086
        ${CONTAINER_ENGINE} run --rm \
            -v "${tmpdir}:/work:Z" \
            ${UBI_IMAGE} bash -c "
                dnf install -y python3-pip git &>/dev/null
                pip install --quiet git+${RPM_LOCKFILE_REPO} 2>/dev/null
                rpm-lockfile-prototype --bare --outfile=/work/rpms.lock.yaml /work/rpms.in.yaml
            " || container_exit=$?
    fi

    if [[ ${container_exit} -ne 0 ]]; then
        error "rpm-lockfile-prototype failed with exit code ${container_exit}"
        return 1
    fi

    if ${use_ubi_repos}; then
        local generated_lock="${tmpdir}/rpms.lock.yaml"
        if [[ ! -f "${generated_lock}" ]]; then
            error "Lockfile was not generated"
            return 1
        fi

        info "Filtering and transforming UBI references to RHEL CDN format..."
        transform_and_filter_lockfile "${generated_lock}" "${RPMS_IN}"
        cp "${generated_lock}" "${RPMS_LOCK}"
    fi

    info "Lockfile updated successfully: ${RPMS_LOCK}"
}

# Show diff if lockfile changed
show_diff() {
    if command -v git &>/dev/null && git rev-parse --is-inside-work-tree &>/dev/null; then
        local diff_output
        diff_output="$(git diff -- "${RPMS_LOCK}" 2>/dev/null || true)"
        if [[ -n "${diff_output}" ]]; then
            info "Changes detected in rpms.lock.yaml:"
            echo "${diff_output}"
        else
            info "No changes detected in rpms.lock.yaml (already up to date)"
        fi
    fi
}

main() {
    info "Updating RPM lockfile..."

    if [[ ! -f "${RPMS_IN}" ]]; then
        error "rpms.in.yaml not found at ${RPMS_IN}"
        exit 1
    fi

    detect_container_engine
    run_lockfile_generator
    show_diff

    info "Done."
}

main "$@"
