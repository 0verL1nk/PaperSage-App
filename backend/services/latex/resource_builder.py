"""Build CLSI resource lists from PaperSage project files."""

from __future__ import annotations

from dataclasses import dataclass

from services.latex.clsi_client import ClsiResource

# Binary file extensions that should use URL references
BINARY_EXTENSIONS = frozenset({
    ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".svg", ".webp",
    ".pdf", ".zip", ".tar", ".gz", ".bz2",
    ".ttf", ".otf", ".woff", ".woff2",
    ".bib", ".bst", ".sty", ".cls", ".fd", ".cfg",
})


@dataclass(frozen=True)
class ProjectFile:
    """Represents a file in a PaperSage project."""

    path: str
    content: str | None = None
    url: str | None = None
    is_binary: bool = False


def build_resources(
    files: list[ProjectFile],
    root_file: str = "main.tex",
) -> tuple[list[ClsiResource], str]:
    """Convert project files to CLSI resources.

    Returns a tuple of (resources list, validated root_resource_path).
    Binary files use URL references; text files use inline content.
    """
    resources: list[ClsiResource] = []
    found_root = False

    for f in files:
        resource = _to_clsi_resource(f)
        resources.append(resource)
        if f.path == root_file:
            found_root = True

    validated_root = root_file if found_root else _infer_root(resources)
    return resources, validated_root


def _to_clsi_resource(f: ProjectFile) -> ClsiResource:
    """Convert a single ProjectFile to a ClsiResource."""
    if f.is_binary or _is_binary_path(f.path):
        return ClsiResource(
            path=f.path,
            url=f.url or "",
            content=None,
        )
    return ClsiResource(
        path=f.path,
        content=f.content or "",
        url=f.url,
    )


def _is_binary_path(path: str) -> bool:
    """Check if a file path has a binary extension."""
    lower = path.lower()
    return any(lower.endswith(ext) for ext in BINARY_EXTENSIONS)


def _infer_root(resources: list[ClsiResource]) -> str:
    """Infer the root resource path from the resource list."""
    for r in resources:
        if r.path.lower().endswith(".tex"):
            return r.path
    return "main.tex"
