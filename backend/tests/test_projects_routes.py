"""Projects API route tests - verification"""
import pytest


def test_projects_router_imports():
    """Verify projects routes can be imported"""
    from api.routes import projects

    assert projects.router is not None
    assert hasattr(projects, "list_projects")
    assert hasattr(projects, "create_project")
    assert hasattr(projects, "get_project")
    assert hasattr(projects, "update_project")
    assert hasattr(projects, "delete_project")


def test_projects_schemas_imports():
    """Verify project schemas can be imported"""
    from schemas import (
        ProjectCreate,
        ProjectUpdate,
        ProjectResponse,
        ProjectGroupCreate,
        ProjectGroupResponse,
        TrashItemResponse,
    )

    # Verify schemas are properly defined
    assert ProjectCreate.model_fields is not None
    assert ProjectUpdate.model_fields is not None
    assert ProjectResponse.model_fields is not None


def test_projects_models_imports():
    """Verify project models can be imported"""
    from models import Project, ProjectGroup

    # Verify models are properly defined
    assert Project.__tablename__ == "projects"
    assert ProjectGroup.__tablename__ == "project_groups"


@pytest.mark.asyncio
async def test_create_project_rejects_empty_name():
    """Verify empty name is rejected"""
    from schemas import ProjectCreate
    from pydantic import ValidationError

    with pytest.raises(ValidationError):
        ProjectCreate(name="")