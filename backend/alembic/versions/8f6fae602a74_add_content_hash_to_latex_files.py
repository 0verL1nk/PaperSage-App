"""add_content_hash_to_latex_files

Revision ID: 8f6fae602a74
Revises: 972ee29fe9c4
Create Date: 2026-04-26 15:02:50.740611
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

revision: str = '8f6fae602a74'
down_revision: Union[str, None] = '972ee29fe9c4'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column('latex_files', sa.Column('content_hash', sa.String(length=64), nullable=True))
    op.create_index(op.f('ix_latex_files_content_hash'), 'latex_files', ['content_hash'], unique=False)


def downgrade() -> None:
    op.drop_index(op.f('ix_latex_files_content_hash'), table_name='latex_files')
    op.drop_column('latex_files', 'content_hash')
