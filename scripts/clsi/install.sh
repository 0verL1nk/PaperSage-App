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
CTAN_OFFICIAL="https://mirror.ctan.org"
CTAN_MIRROR="https://mirrors.tuna.tsinghua.edu.cn/CTAN"

# Pick CTAN mirror: use official if reachable, otherwise fall back to China mirror
pick_ctan_mirror() {
    info "Testing CTAN connectivity ..."
    if curl -fsSL --connect-timeout 5 --max-time 10 \
        "${CTAN_OFFICIAL}/systems/texlive/tlnet/" -o /dev/null 2>/dev/null; then
        ok "Official CTAN reachable, using mirror.ctan.org"
        SELECTED_CTAN="${CTAN_OFFICIAL}"
    else
        info "Official CTAN unreachable, using Tsinghua mirror"
        SELECTED_CTAN="${CTAN_MIRROR}"
    fi
}

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

# ─── TeX Live (upstream installer, no sudo) ───

ensure_texlive_debian() {
    if has_cmd pdflatex && has_cmd xelatex && has_cmd lualatex; then
        ok "TeX Live engines present"
        return 0
    fi

    if $CHECK_ONLY; then
        fail "TeX Live not found"
        return 1
    fi

    local year
    year=$(date +%Y)
    local TEXDIR="${HOME}/.local/texlive/${year}"
    local tmpdir
    tmpdir=$(mktemp -d)
    local cleanup_on_interrupt=true

    _texlive_cleanup() {
        if $cleanup_on_interrupt; then
            echo ""
            warn "Interrupted, cleaning up ..."
            kill "$(jobs -p)" 2>/dev/null
            rm -rf "${tmpdir}"
            exit 130
        fi
    }
    trap _texlive_cleanup INT TERM

    info "Installing TeX Live via upstream installer ..."

    pick_ctan_mirror

    # Download installer
    info "Downloading install-tl ..."
    curl -fsSL "${SELECTED_CTAN}/systems/texlive/tlnet/install-tl-unx.tar.gz" \
        -o "${tmpdir}/install-tl-unx.tar.gz"

    tar xzf "${tmpdir}/install-tl-unx.tar.gz" -C "${tmpdir}"
    local installer_dir
    installer_dir=$(find "${tmpdir}" -maxdepth 1 -type d -name "install-tl-*" | head -1)

    if [[ -z "${installer_dir}" ]]; then
        rm -rf "${tmpdir}"
        fail "Could not find install-tl in extracted archive"
        return 1
    fi

    # Create profile: scheme-full for compilation service, install to user directory
    cat > "${tmpdir}/texlive.profile" << EOF
selected_scheme scheme-full
TEXDIR ${TEXDIR}
TEXMFLOCAL ${TEXDIR}/texmf-local
TEXMFSYSCONFIG ${TEXDIR}/texmf-config
TEXMFSYSVAR ${TEXDIR}/texmf-var
TEXMFHOME ~/texmf
TEXMFCONFIG ~/.texlive/${year}/texmf-config
TEXMFVAR ~/.texlive/${year}/texmf-var
instopt_adjustpath 0
instopt_adjustrepo 1
EOF

    # Run installer
    info "Installing TeX Live (scheme-full, this will take a while) ..."
    "${installer_dir}/install-tl" \
        --profile="${tmpdir}/texlive.profile" \
        --repository="${SELECTED_CTAN}/systems/texlive/tlnet"

    rm -rf "${tmpdir}"
    cleanup_on_interrupt=false
    trap - INT TERM
    local TEXBIN
    TEXBIN=$(find "${TEXDIR}/bin" -maxdepth 1 -type d ! -path "${TEXDIR}/bin" | head -1)

    if [[ -z "${TEXBIN}" ]]; then
        fail "Could not find TeX Live bin directory after installation"
        return 1
    fi

    # Add to PATH in shell config
    local rc_file="${HOME}/.zshrc"
    if ! grep -q "${TEXBIN}" "${rc_file}" 2>/dev/null; then
        printf '\n# TeX Live\nexport PATH="%s:$PATH"\n' "${TEXBIN}" >> "${rc_file}"
        info "Added ${TEXBIN} to PATH in ${rc_file}"
    fi
    export PATH="${TEXBIN}:${PATH}"

    ok "TeX Live installed (scheme-full, no sudo)"
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
