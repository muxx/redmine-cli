#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:?VERSION is required}"
REPOSITORY="${GITHUB_REPOSITORY:-muxx/redmine-cli}"
DIST_DIR="${DIST_DIR:-dist}"
TAP_REPOSITORY="${HOMEBREW_TAP_REPOSITORY:-muxx/homebrew-tap}"
FORMULA_NAME="${HOMEBREW_FORMULA_NAME:-redmine-cli}"
FORMULA_CLASS="${HOMEBREW_FORMULA_CLASS:-RedmineCli}"
FORMULA_PATH="Formula/${FORMULA_NAME}.rb"

version_number="${VERSION#v}"

archive_name() {
  local goos="$1"
  local goarch="$2"
  case "${goos}" in
    windows)
      printf 'redmine_%s_%s_%s.zip' "${VERSION}" "${goos}" "${goarch}"
      ;;
    *)
      printf 'redmine_%s_%s_%s.tar.gz' "${VERSION}" "${goos}" "${goarch}"
      ;;
  esac
}

archive_sha() {
  local archive="$1"
  awk -v archive="${archive}" '$2 == archive { print $1 }' "${DIST_DIR}/checksums.txt"
}

download_url() {
  local archive="$1"
  printf 'https://github.com/%s/releases/download/%s/%s' "${REPOSITORY}" "${VERSION}" "${archive}"
}

required_archive_sha() {
  local archive="$1"
  local sha
  sha="$(archive_sha "${archive}")"
  if [[ -z "${sha}" ]]; then
    echo "missing checksum for ${archive}" >&2
    exit 1
  fi
  printf '%s' "${sha}"
}

darwin_amd64_archive="$(archive_name darwin amd64)"
darwin_arm64_archive="$(archive_name darwin arm64)"
linux_amd64_archive="$(archive_name linux amd64)"
linux_arm64_archive="$(archive_name linux arm64)"

darwin_amd64_sha="$(required_archive_sha "${darwin_amd64_archive}")"
darwin_arm64_sha="$(required_archive_sha "${darwin_arm64_archive}")"
linux_amd64_sha="$(required_archive_sha "${linux_amd64_archive}")"
linux_arm64_sha="$(required_archive_sha "${linux_arm64_archive}")"

tap_dir="${HOMEBREW_TAP_DIR:-}"
if [[ -z "${tap_dir}" ]]; then
  if [[ -z "${GH_TOKEN:-}" ]]; then
    echo "GH_TOKEN with write access to ${TAP_REPOSITORY} is required" >&2
    exit 1
  fi
  tap_dir="$(mktemp -d)"
  trap 'rm -rf "${tap_dir}"' EXIT
  git clone "https://x-access-token:${GH_TOKEN}@github.com/${TAP_REPOSITORY}.git" "${tap_dir}"
else
  mkdir -p "${tap_dir}"
fi

mkdir -p "${tap_dir}/Formula"
cat >"${tap_dir}/${FORMULA_PATH}" <<EOF
class ${FORMULA_CLASS} < Formula
  desc "Redmine CLI generated from the Redmine OpenAPI specification"
  homepage "https://github.com/${REPOSITORY}"
  version "${version_number}"
  license "MIT"

  on_macos do
    on_arm do
      url "$(download_url "${darwin_arm64_archive}")"
      sha256 "${darwin_arm64_sha}"
    end

    on_intel do
      url "$(download_url "${darwin_amd64_archive}")"
      sha256 "${darwin_amd64_sha}"
    end
  end

  on_linux do
    on_arm do
      url "$(download_url "${linux_arm64_archive}")"
      sha256 "${linux_arm64_sha}"
    end

    on_intel do
      url "$(download_url "${linux_amd64_archive}")"
      sha256 "${linux_amd64_sha}"
    end
  end

  def install
    bin.install "redmine"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/redmine --version")
  end
end
EOF

if [[ -n "${HOMEBREW_TAP_DIR:-}" ]]; then
  echo "Formula written to ${tap_dir}/${FORMULA_PATH}"
  exit 0
fi

cd "${tap_dir}"
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
git add "${FORMULA_PATH}"
if git diff --cached --quiet; then
  echo "Homebrew formula is already current"
  exit 0
fi

git commit -m "Update ${FORMULA_NAME} to ${VERSION}"
git push
