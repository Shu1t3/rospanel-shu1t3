#!/usr/bin/env bash
# scripts/generate-release-notes.sh
# Generates bilingual (RU / EN) release notes for RosPanel releases.
# Usage:
#   ./scripts/generate-release-notes.sh <tag> [output_file]
# Example:
#   ./scripts/generate-release-notes.sh v5.12.0 RELEASE_NOTES.md

set -euo pipefail

TAG="${1:-}"
OUTPUT_FILE="${2:-}"

if [ -z "$TAG" ]; then
  # Try to detect latest tag from git or version file
  if [ -f "internal/version/version.go" ]; then
    CURR_VER=$(grep 'Version = ' internal/version/version.go | cut -d'"' -f2)
    TAG="v${CURR_VER}"
  else
    TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
  fi
fi

RAW_VER="${TAG#v}"

# Extract changelog section for this version if CHANGELOG.md exists
CHANGELOG_SNIPPET=""
if [ -f "CHANGELOG.md" ]; then
  CHANGELOG_SNIPPET=$(awk -v ver="$RAW_VER" '
    $0 ~ "^## \\[" ver "\\]" || $0 ~ "^## " ver { p=1; next }
    p && /^## / { p=0 }
    p { print }
  ' CHANGELOG.md || true)
fi

# Fallback to git log if changelog section empty
if [ -z "$(echo "$CHANGELOG_SNIPPET" | tr -d '[:space:]')" ]; then
  PREV_TAG=$(git describe --tags --abbrev=0 "${TAG}^" 2>/dev/null || git rev-list --max-parents=0 HEAD)
  CHANGELOG_SNIPPET=$(git log --pretty=format:"* %s (%h)" "${PREV_TAG}..${TAG}" 2>/dev/null || git log -n 10 --pretty=format:"* %s (%h)")
fi

TMP_OUT=$(mktemp)

cat <<EOF > "$TMP_OUT"
## RosPanel ${TAG}

### 🇷🇺 Русский

#### Что нового
${CHANGELOG_SNIPPET}

#### Инварианты форка (Fork Safeguards)
- **Мульти-хоп маршрутизация**: VLESS цепочки (вход → транзит → выход), выделенные порты, обратные маршруты сохранены.
- **Лимиты устройств**: отслеживание активных сессий и принудительное завершение соединений активны.
- **REALITY & Decoy**: расширенный пул сайтов РФ и twin names функционируют в штатном режиме.
- **Безопасность**: защита от подбора на nftables, идемпотентные миграции SQLite WAL.

#### Обновление
\`\`\`bash
# На работающем сервере:
rospanel update

# Либо переустановка последней версии:
curl -fsSL https://raw.githubusercontent.com/Shu1t3/rospanel-shu1t3/main/install.sh | bash
\`\`\`

---

### 🇬🇧 English

#### Highlights & Changes
${CHANGELOG_SNIPPET}

#### Fork Safeguards Maintained
- **Multi-Hop Routing**: VLESS multi-hop chains (entry → transit → exit) with reverse routing.
- **Device Limits**: Active session tracking and subscription token rotation active.
- **REALITY & Decoy**: Extended pool of RU decoy domains and twin names operational.
- **Security & Storage**: nftables brute-force guard and idempotent SQLite WAL migrations.

#### Upgrade Instructions
\`\`\`bash
# On a running instance:
rospanel update

# Or clean install:
curl -fsSL https://raw.githubusercontent.com/Shu1t3/rospanel-shu1t3/main/install.sh | bash
\`\`\`
EOF

if [ -n "$OUTPUT_FILE" ]; then
  mv "$TMP_OUT" "$OUTPUT_FILE"
  echo "Release notes written to ${OUTPUT_FILE}"
else
  cat "$TMP_OUT"
  rm -f "$TMP_OUT"
fi
