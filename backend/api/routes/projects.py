"""Projects routes."""

import uuid
from datetime import UTC, datetime

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from api.dependencies import get_db
from api.response import ApiResponse, api_success
from api.routes.auth import get_current_user
from models import Project, ProjectGroup, User
from schemas import (
    ProjectCreate,
    ProjectGroupCreate,
    ProjectGroupResponse,
    ProjectGroupUpdate,
    ProjectUpdate,
    ProjectWithGroupResponse,
    TrashItemResponse,
)

router = APIRouter()


# ============ Projects ============
# IMPORTANT: Routes with specific paths MUST come BEFORE /{project_id}
# FastAPI matches routes in order of definition


@router.get("/", response_model=ApiResponse[list[ProjectWithGroupResponse]])
async def list_projects(
    group_id: uuid.UUID | None = None,
    include_deleted: bool = False,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[list[ProjectWithGroupResponse]]:
    """List all projects for the current user."""
    query = select(Project).where(Project.owner_id == current_user.id)

    if group_id:
        query = query.where(Project.group_id == group_id)

    if not include_deleted:
        query = query.where(Project.is_deleted == False)  # noqa: E712

    query = query.options(selectinload(Project.group)).order_by(Project.updated_at.desc())

    result = await db.execute(query)
    projects = result.scalars().all()

    return api_success([ProjectWithGroupResponse.model_validate(p) for p in projects])


@router.post("/", response_model=ApiResponse[ProjectWithGroupResponse])
async def create_project(
    project_data: ProjectCreate,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[ProjectWithGroupResponse]:
    """Create a new project."""
    # Check for duplicate name under the same user
    existing = await db.execute(
        select(Project).where(
            Project.owner_id == current_user.id,
            Project.name == project_data.name,
            not Project.is_deleted,
        )
    )
    if existing.scalar_one_or_none():
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Project with this name already exists",
        )

    # Verify group belongs to user if provided
    if project_data.group_id:
        result = await db.execute(
            select(ProjectGroup).where(
                ProjectGroup.id == project_data.group_id,
                ProjectGroup.owner_id == current_user.id,
            )
        )
        if not result.scalar_one_or_none():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Group not found or does not belong to user",
            )

    project = Project(
        name=project_data.name,
        description=project_data.description,
        owner_id=current_user.id,
        group_id=project_data.group_id,
        status="active",
    )

    db.add(project)
    await db.commit()
    await db.refresh(project)

    # Create default main.tex template
    from services.latex.file_service import LatexFileService

    default_tex = (
        "\\documentclass{article}\n"
        "\\usepackage[utf8]{inputenc}\n\n"
        "\\title{New Project}\n"
        "\\author{}\n"
        "\\date{\\today}\n\n"
        "\\begin{document}\n\n"
        "\\maketitle\n\n"
        "\\section{Introduction}\n\n"
        "Write your content here.\n\n"
        "\\end{document}"
    )
    try:
        file_service = LatexFileService()
        await file_service.create_file(project.id, "main.tex", default_tex, current_user.id, db)
    except Exception:
        pass  # Non-critical: template creation failure should not block project creation

    # Load group relationship
    result = await db.execute(
        select(Project).where(Project.id == project.id).options(selectinload(Project.group))
    )
    project = result.scalar_one()

    return api_success(ProjectWithGroupResponse.model_validate(project))


# ============ Groups ============
# These routes MUST come before /{project_id} to avoid route conflicts


@router.get("/groups", response_model=ApiResponse[list[ProjectGroupResponse]])
async def list_groups(
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[list[ProjectGroupResponse]]:
    """List all project groups for the current user."""
    result = await db.execute(
        select(ProjectGroup)
        .where(ProjectGroup.owner_id == current_user.id)
        .order_by(ProjectGroup.sort_order, ProjectGroup.name)
    )
    groups = result.scalars().all()

    return api_success([ProjectGroupResponse.model_validate(g) for g in groups])


@router.post("/groups", response_model=ApiResponse[ProjectGroupResponse])
async def create_group(
    group_data: ProjectGroupCreate,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[ProjectGroupResponse]:
    """Create a new project group."""
    # Get max sort_order
    result = await db.execute(select(ProjectGroup).where(ProjectGroup.owner_id == current_user.id))
    groups = result.scalars().all()
    max_order = max((g.sort_order for g in groups), default=-1)

    group = ProjectGroup(
        name=group_data.name,
        owner_id=current_user.id,
        sort_order=max_order + 1,
    )

    db.add(group)
    await db.commit()
    await db.refresh(group)

    return api_success(ProjectGroupResponse.model_validate(group))


@router.put("/groups/{group_id}", response_model=ApiResponse[ProjectGroupResponse])
async def update_group(
    group_id: uuid.UUID,
    group_data: ProjectGroupUpdate,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[ProjectGroupResponse]:
    """Update a project group."""
    result = await db.execute(
        select(ProjectGroup).where(
            ProjectGroup.id == group_id,
            ProjectGroup.owner_id == current_user.id,
        )
    )
    group = result.scalar_one_or_none()

    if not group:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Group not found",
        )

    if group_data.name is not None:
        group.name = group_data.name
    if group_data.sort_order is not None:
        group.sort_order = group_data.sort_order

    await db.commit()
    await db.refresh(group)

    return api_success(ProjectGroupResponse.model_validate(group))


@router.delete("/groups/{group_id}")
async def delete_group(
    group_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Delete a project group (moves projects to ungrouped)."""
    result = await db.execute(
        select(ProjectGroup).where(
            ProjectGroup.id == group_id,
            ProjectGroup.owner_id == current_user.id,
        )
    )
    group = result.scalar_one_or_none()

    if not group:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Group not found",
        )

    # Move projects to ungrouped
    result = await db.execute(
        select(Project).where(
            Project.group_id == group_id,
            Project.owner_id == current_user.id,
            not Project.is_deleted,
        )
    )
    projects = result.scalars().all()
    for project in projects:
        project.group_id = None

    await db.delete(group)
    await db.commit()

    return api_success({"id": str(group_id), "deleted": True})


# ============ Trash ============
# These routes MUST come before /{project_id} to avoid route conflicts


@router.get("/trash", response_model=ApiResponse[list[TrashItemResponse]])
async def list_trash(
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[list[TrashItemResponse]]:
    """List all deleted projects in trash."""
    result = await db.execute(
        select(Project).where(
            Project.owner_id == current_user.id,
            Project.is_deleted,
        )
    )
    projects = result.scalars().all()

    trash_items = [
        TrashItemResponse(
            type="project",
            id=p.id,
            name=p.name,
            deleted_at=datetime.fromisoformat(p.deleted_at) if p.deleted_at else datetime.now(UTC),
            original_data={"description": p.description, "status": p.status},
        )
        for p in projects
    ]

    return api_success(trash_items)


@router.post("/trash/project/{project_id}/restore")
async def restore_project(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Restore a deleted project from trash."""
    result = await db.execute(
        select(Project).where(
            Project.id == project_id,
            Project.owner_id == current_user.id,
            Project.is_deleted,
        )
    )
    project = result.scalar_one_or_none()

    if not project:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Project not found in trash",
        )

    project.is_deleted = False
    project.deleted_at = None

    await db.commit()

    return api_success({"id": str(project_id), "restored": True})


@router.delete("/trash/project/{project_id}")
async def permanently_delete_project(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Permanently delete a project from trash."""
    result = await db.execute(
        select(Project).where(
            Project.id == project_id,
            Project.owner_id == current_user.id,
            Project.is_deleted,
        )
    )
    project = result.scalar_one_or_none()

    if not project:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Project not found in trash",
        )

    await db.delete(project)
    await db.commit()

    return api_success({"id": str(project_id), "permanently_deleted": True})


# ============ Project CRUD by ID ============
# These routes come LAST because they have a path parameter


@router.get("/{project_id}", response_model=ApiResponse[ProjectWithGroupResponse])
async def get_project(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[ProjectWithGroupResponse]:
    """Get a project by ID."""
    result = await db.execute(
        select(Project)
        .where(Project.id == project_id, Project.owner_id == current_user.id)
        .options(selectinload(Project.group))
    )
    project = result.scalar_one_or_none()

    if not project:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Project not found",
        )

    return api_success(ProjectWithGroupResponse.model_validate(project))


@router.put("/{project_id}", response_model=ApiResponse[ProjectWithGroupResponse])
async def update_project(
    project_id: uuid.UUID,
    project_data: ProjectUpdate,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[ProjectWithGroupResponse]:
    """Update a project."""
    result = await db.execute(
        select(Project)
        .where(Project.id == project_id, Project.owner_id == current_user.id)
        .options(selectinload(Project.group))
    )
    project = result.scalar_one_or_none()

    if not project:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Project not found",
        )

    # Check for duplicate name under the same user (excluding current project)
    if project_data.name is not None and project_data.name != project.name:
        existing = await db.execute(
            select(Project).where(
                Project.owner_id == current_user.id,
                Project.name == project_data.name,
                Project.id != project_id,
                not Project.is_deleted,
            )
        )
        if existing.scalar_one_or_none():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Project with this name already exists",
            )

    # Verify group belongs to user if provided
    if project_data.group_id is not None:
        if project_data.group_id:
            group_result = await db.execute(
                select(ProjectGroup).where(
                    ProjectGroup.id == project_data.group_id,
                    ProjectGroup.owner_id == current_user.id,
                )
            )
            if not group_result.scalar_one_or_none():
                raise HTTPException(
                    status_code=status.HTTP_400_BAD_REQUEST,
                    detail="Group not found or does not belong to user",
                )
        project.group_id = project_data.group_id

    if project_data.name is not None:
        project.name = project_data.name
    if project_data.description is not None:
        project.description = project_data.description
    if project_data.status is not None:
        project.status = project_data.status

    await db.commit()
    await db.refresh(project)

    return api_success(ProjectWithGroupResponse.model_validate(project))


@router.delete("/{project_id}")
async def delete_project(
    project_id: uuid.UUID,
    current_user: User = Depends(get_current_user),  # noqa: B008
    db: AsyncSession = Depends(get_db),  # noqa: B008
) -> ApiResponse[dict]:
    """Soft delete a project (move to trash)."""
    result = await db.execute(
        select(Project).where(
            Project.id == project_id,
            Project.owner_id == current_user.id,
            not Project.is_deleted,
        )
    )
    project = result.scalar_one_or_none()

    if not project:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Project not found",
        )

    project.is_deleted = True
    project.deleted_at = datetime.now(UTC).isoformat()

    await db.commit()

    return api_success({"id": str(project_id), "deleted": True})
