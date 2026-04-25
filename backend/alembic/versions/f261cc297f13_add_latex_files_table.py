"""add latex_files table

Revision ID: f261cc297f13
Revises: d94d978b402a
Create Date: 2026-04-25 14:00:18.217380

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'f261cc297f13'
down_revision: Union[str, None] = 'd94d978b402a'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        'latex_files',
        sa.Column('id', sa.String(32), nullable=False),
        sa.Column('project_id', sa.String(32), nullable=False),
        sa.Column('file_path', sa.String(500), nullable=False),
        sa.Column('content_type', sa.String(100), nullable=True),
        sa.Column('size_bytes', sa.Integer(), nullable=False, server_default='0'),
        sa.Column('s3_key', sa.String(500), nullable=True),
        sa.Column('last_saved_at', sa.DateTime(), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.Column('updated_at', sa.DateTime(), nullable=False),
        sa.ForeignKeyConstraint(['project_id'], ['projects.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('project_id', 'file_path', name='uq_project_file_path'),
    )


def downgrade() -> None:
    op.drop_table('latex_files')
