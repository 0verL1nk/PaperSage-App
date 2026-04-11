"""Documents routes (placeholder for Phase 3)."""
from fastapi import APIRouter

router = APIRouter()


@router.get("/")
async def list_documents():
    """List documents - to be implemented in Phase 3."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.post("/")
async def create_document():
    """Create document - to be implemented in Phase 3."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.get("/{document_id}")
async def get_document(document_id: str):
    """Get document - to be implemented in Phase 3."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.put("/{document_id}")
async def update_document(document_id: str):
    """Update document - to be implemented in Phase 3."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}


@router.delete("/{document_id}")
async def delete_document(document_id: str):
    """Delete document - to be implemented in Phase 3."""
    return {"code": 501, "data": None, "info": "Not implemented yet"}
