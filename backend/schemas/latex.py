"""LaTeX schemas for request/response validation."""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field

# Compiler options
COMPILER_PDFLATEX = "pdflatex"
COMPILER_XELATEX = "xelatex"
COMPILER_LUALATEX = "lualatex"
VALID_COMPILERS = {COMPILER_PDFLATEX, COMPILER_XELATEX, COMPILER_LUALATEX}


class LatexFileInput(BaseModel):
    """A single file input for compilation."""

    path: str = Field(..., min_length=1, description="File path relative to project root")
    content: str | None = Field(None, description="Text file content (for .tex/.cls etc.)")
    url: str | None = Field(None, description="URL reference (for binary files)")
    is_binary: bool = Field(False, description="Whether this is a binary file")


class LatexCompileRequest(BaseModel):
    """Request to compile a LaTeX project."""

    root_file: str = Field("main.tex", description="Root .tex file path")
    compiler: str = Field(COMPILER_PDFLATEX, description="TeX compiler to use")
    files: list[LatexFileInput] = Field(default_factory=list, description="Project files (empty to use stored files)")


class LatexOutputFileResponse(BaseModel):
    """Output file metadata in compile response."""

    path: str
    url: str
    type: str = ""
    build: str = ""
    size: int = 0


class LatexCompileResponse(BaseModel):
    """Response from a compile request."""

    status: str
    build_id: str | None = None
    output_files: list[LatexOutputFileResponse] = Field(default_factory=list)
    error: str | None = None
    logs: str | None = None


class LatexWordcountResponse(BaseModel):
    """Response from a wordcount request."""

    texcount: dict[str, Any] = Field(default_factory=dict)


class LatexSyncCodeRequest(BaseModel):
    """Request for SyncTeX source-to-PDF lookup."""

    file: str = Field(..., description="Source file path")
    line: int = Field(..., ge=0, description="Line number")
    column: int = Field(..., ge=0, description="Column number")


class LatexSyncPdfRequest(BaseModel):
    """Request for SyncTeX PDF-to-source lookup."""

    page: int = Field(..., ge=1, description="PDF page number")
    h: float = Field(..., description="Horizontal position")
    v: float = Field(..., description="Vertical position")


class LatexSyncPosition(BaseModel):
    """A single position in SyncTeX response."""

    file: str | None = None
    line: int | None = None
    column: int | None = None
    page: int | None = None
    h: float | None = None
    v: float | None = None
    width: float | None = None
    height: float | None = None


class LatexSyncCodeResponse(BaseModel):
    """Response from SyncTeX source-to-PDF lookup."""

    pdf: list[LatexSyncPosition] = Field(default_factory=list)


class LatexSyncPdfResponse(BaseModel):
    """Response from SyncTeX PDF-to-source lookup."""

    code: list[LatexSyncPosition] = Field(default_factory=list)
