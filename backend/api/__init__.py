"""API routes."""

from fastapi import APIRouter

from api.routes import auth, documents, latex, latex_files, projects

api_router = APIRouter()

api_router.include_router(auth.router, prefix="/auth", tags=["auth"])
api_router.include_router(documents.router, prefix="/documents", tags=["documents"])
api_router.include_router(projects.router, prefix="/projects", tags=["projects"])
api_router.include_router(
    latex.router, prefix="/projects/{project_id}/latex", tags=["latex"]
)
api_router.include_router(
    latex_files.router,
    prefix="/projects/{project_id}/latex-files",
    tags=["latex-files"],
)
