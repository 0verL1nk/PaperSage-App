#!/usr/bin/env bash
set -euo pipefail

# ─── PaperSage CLSI Host-Mode Prerequisites Installer ───
# Supports: Ubuntu/Debian, macOS
# Usage:
#   ./install.sh          # Install missing dependencies
#   ./install.sh --check  # Only verify, do not install

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
LATEX_VENDER="${PROJECT_ROOT}/latex_render"

# Minimum versions
NODE_MAJOR=20
TEXLIVE_PACKAGES="texlive-latex-recommended texlive-latex-extra texlive-fonts-recommended texlive-fonts-extra texlive-science texlive-bibtex-extra texlive-xetex texlive-luatex texlive-extra-utils"

# ─── Helpers ───

info()  { printf "\033[0;34m[INFO]\033[0m  %s\n" "$*"; }
ok()    { printf "\033[0;32m[OK]\033[0m    %s\n" "$*"; }
warn()  { printf "\033[0;33m[WARN]\033[0m  %s\n" "$*"; }
fail()  { printf "\033[0;31m[FAIL]\033[0m  %s\n" "$*" >&2; }

CHECK_ONLY=false
if [[ "${1:-}" == "--check" ]]; then
    CHECK_ONLY=true
    info "Running in check-only mode (no installations will be performed)"
fi

has_cmd() { command -v "$1" &>/dev/null; }

detect_os() {
    if [[ "$(uname -s)" == "Darwin" ]]; then
        echo "macos"
    elif [[ -f /etc/debian_version ]] || [[ -f /etc/os-release ]] && grep -qiE "ubuntu|debian" /etc/os-release 2>/dev/null; then
        echo "debian"
    else
        echo "unknown"
    fi
}

# ─── Node.js ───

ensure_node() {
    if has_cmd node && has_cmd npm; then
        local ver
        ver=$(node -v | sed 's/^v//' | cut -d. -f1)
        if (( ver >= NODE_MAJOR )); then
            ok "Node.js $(node -v)"
            return 0
        fi
        warn "Node.js $(node -v) found, but >= v${NODE_MAJOR} required"
    fi

    if $CHECK_ONLY; then
        fail "Node.js >= v${NODE_MAJOR} not found"
        return 1
    fi

    info "Installing Node.js ..."
    local os
    os=$(detect_os)
    case "$os" in
        debian)
            if ! has_cmd curl; then
                sudo apt-get update -qq && sudo apt-get install -y -qq curl
            fi
            curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | sudo -E bash -
            sudo apt-get install -y nodejs
            ;;
        macos)
            if has_cmd brew; then
                brew install node@${NODE_MAJOR}
                brew link --overwrite node@${NODE_MAJOR} 2>/dev/null || true
            else
                fail "Homebrew not found. Install from https://brew.sh"
                return 1
            fi
            ;;
        *)
            fail "Unsupported OS for automatic Node.js install"
            return 1
            ;;
    esac
    ok "Node.js $(node -v) installed"
}

# ─── TeX Live (Debian/Ubuntu) ───

ensure_texlive_debian() {
    if has_cmd pdflatex && has_cmd xelatex && has_cmd lualatex; then
        ok "TeX Live engines present"
        return 0
    fi

    if $CHECK_ONLY; then
        fail "TeX Live not found"
        return 1
    fi

    info "Installing TeX Live (this may take a while) ..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq ${TEXLIVE_PACKAGES}
    ok "TeX Live installed"
}

# ─── TeX Live (macOS) ───

ensure_texlive_macos() {
    if has_cmd pdflatex && has_cmd xelatex && has_cmd lualatex; then
        ok "TeX Live engines present"
        return 0
    fi

    if $CHECK_ONLY; then
        fail "TeX Live not found"
        return 1
    fi

    info "Installing TeX Live via Homebrew (basictex) ..."
    brew install --cask basictex

    local texbin="/Library/TeX/texbin"
    export PATH="${texbin}:${PATH}"
    if ! grep -q "${texbin}" "${HOME}/.zshrc" 2>/dev/null; then
        printf '\nexport PATH="%s:\$PATH"\n' "${texbin}" >> "${HOME}/.zshrc"
        info "Added ${texbin} to PATH in ~/.zshrc (restart shell to apply)"
    fi

    sudo tlmgr update --self
    sudo tlmgr install latexmk collection-latexextra collection-fontsrecommended \
        collection-fontsextra collection-science collection-bibtexextra \
        collection-xetex collection-luatex

    ok "TeX Live installed (basictex + extras)"
}

# ─── Auxiliary tools ───

ensure_aux_tools() {
    local tools=(latexmk biber qpdf)
    local missing=()

    for tool in "${tools[@]}"; do
        if has_cmd "$tool"; then
            ok "$tool available"
        else
            missing+=("$tool")
        fi
    done

    # inkscape is optional for image conversion
    if has_cmd inkscape; then
        ok "inkscape available"
    else
        missing+=("inkscape")
        warn "inkscape not found (optional for SVG conversion)"
    fi

    if [[ ${#missing[@]} -eq 0 ]]; then
        return 0
    fi

    if $CHECK_ONLY; then
        for tool in "${missing[@]}"; do
            fail "$tool not found"
        done
        return 1
    fi

    local os
    os=$(detect_os)
    case "$os" in
        debian) sudo apt-get install -y -qq "${missing[@]}" ;;
        macos)  brew install "${missing[@]}" ;;
        *)      fail "Cannot install ${missing[*]} on unknown OS"; return 1 ;;
    esac
    ok "Auxiliary tools installed: ${missing[*]}"
}

# ─── Node workspace dependencies ───

ensure_npm_deps() {
    if [[ -d "${LATEX_VENDER}/node_modules" ]]; then
        ok "Node workspace dependencies present"
        return 0
    fi

    if $CHECK_ONLY; then
        fail "Node workspace dependencies not installed (run without --check)"
        return 1
    fi

    info "Installing vendored workspace dependencies ..."
    (cd "${LATEX_VENDER}" && npm install --ignore-scripts)
    ok "Node workspace dependencies installed"
}

# ─── Main ───

main() {
    local os
    os=$(detect_os)

    if [[ "$os" == "unknown" ]]; then
        fail "Unsupported operating system. This script supports Ubuntu/Debian and macOS."
        exit 1
    fi

    info "Detected OS: ${os}"

    ensure_node

    case "$os" in
        debian) ensure_texlive_debian ;;
        macos)  ensure_texlive_macos ;;
    esac

    ensure_aux_tools
    ensure_npm_deps

    echo ""
    if $CHECK_ONLY; then
        info "Check complete. Run without --check to install missing dependencies."
    else
        ok "All CLSI host-mode prerequisites installed."
        info "Run 'make check-clsi' to verify, then 'make clsi-dev' to start."
    fi
}

main "$@"
