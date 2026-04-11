"""Projects routes (placeholder for Phase 4)."""
from fastapi import APIRouter

router = APIRouter()


@router.get("/")
async def list_projects():
    """List projects - to be implemented in Phase 4."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.post("/")
async def create_project():
    """Create project - to be implemented in Phase 4."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.get("/{project_id}")
async def get_project(project_id: str):
    """Get project - to be implemented in Phase 4."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.put("/{project_id}")
async def update_project(project_id: str):
    """Update project - to be implemented in Phase 4."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.delete("/{project_id}")
async def delete_project(project_id: str):
    """Delete project - to be implemented in Phase 4."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}
