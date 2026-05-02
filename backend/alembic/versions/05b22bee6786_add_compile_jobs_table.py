"""add compile_jobs table

Revision ID: 05b22bee6786
Revises: 8f6fae602a74
Create Date: 2026-04-26 18:47:40.237208
"""
from typing import Sequence, Union

import sqlalchemy as sa
from alembic import op

from core.base import UUID

# revision identifiers, used by Alembic.
revision: str = "05b22bee6786"
down_revision: Union[str, None] = "8f6fae602a74"
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        "compile_jobs",
        sa.Column("project_id", UUID(), nullable=False),
        sa.Column("user_id", UUID(), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("root_file", sa.String(length=500), nullable=False),
        sa.Column("compiler", sa.String(length=20), nullable=False),
        sa.Column("build_id", sa.String(length=100), nullable=True),
        sa.Column("error", sa.Text(), nullable=True),
        sa.Column("logs", sa.Text(), nullable=True),
        sa.Column("id", UUID(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(["project_id"], ["projects.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["user_id"], ["users.id"], ondelete="CASCADE"),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index(op.f("ix_compile_jobs_project_id"), "compile_jobs", ["project_id"], unique=False)
    op.create_index(op.f("ix_compile_jobs_status"), "compile_jobs", ["status"], unique=False)
    op.create_index(op.f("ix_compile_jobs_user_id"), "compile_jobs", ["user_id"], unique=False)


def downgrade() -> None:
    op.drop_index(op.f("ix_compile_jobs_user_id"), table_name="compile_jobs")
    op.drop_index(op.f("ix_compile_jobs_status"), table_name="compile_jobs")
    op.drop_index(op.f("ix_compile_jobs_project_id"), table_name="compile_jobs")
    op.drop_table("compile_jobs")
