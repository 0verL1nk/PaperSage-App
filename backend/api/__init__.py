"""API routes."""
from fastapi import APIRouter

from api.routes import auth, documents, projects

api_router = APIRouter()

api_router.include_router(auth.router, prefix="/v1/auth", tags=["auth"])
api_router.include_router(documents.router, prefix="/documents", tags=["documents"])
api_router.include_router(projects.router, prefix="/projects", tags=["projects"])
