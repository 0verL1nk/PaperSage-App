"""add_folders_and_documents_tables

Revision ID: 972ee29fe9c4
Revises: f261cc297f13
Create Date: 2026-04-26 10:58:03.991052

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

import core.base

# revision identifiers, used by Alembic.
revision: str = '972ee29fe9c4'
down_revision: Union[str, None] = 'f261cc297f13'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table('folders',
    sa.Column('name', sa.String(length=100), nullable=False),
    sa.Column('parent_id', core.base.UUID(length=32), nullable=True),
    sa.Column('project_id', core.base.UUID(length=32), nullable=False),
    sa.Column('owner_id', core.base.UUID(length=32), nullable=False),
    sa.Column('id', core.base.UUID(length=32), nullable=False),
    sa.Column('created_at', sa.DateTime(timezone=True), nullable=False),
    sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False),
    sa.ForeignKeyConstraint(['owner_id'], ['users.id'], ondelete='CASCADE'),
    sa.ForeignKeyConstraint(['parent_id'], ['folders.id'], ondelete='CASCADE'),
    sa.ForeignKeyConstraint(['project_id'], ['projects.id'], ondelete='CASCADE'),
    sa.PrimaryKeyConstraint('id')
    )
    op.create_table('documents',
    sa.Column('name', sa.String(length=255), nullable=False),
    sa.Column('type', sa.String(length=100), nullable=False),
    sa.Column('size', sa.BigInteger(), nullable=False),
    sa.Column('status', sa.String(length=20), nullable=False),
    sa.Column('s3_key', sa.String(length=500), nullable=False),
    sa.Column('remark', sa.Text(), nullable=True),
    sa.Column('project_id', core.base.UUID(length=32), nullable=False),
    sa.Column('folder_id', core.base.UUID(length=32), nullable=True),
    sa.Column('owner_id', core.base.UUID(length=32), nullable=False),
    sa.Column('id', core.base.UUID(length=32), nullable=False),
    sa.Column('created_at', sa.DateTime(timezone=True), nullable=False),
    sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False),
    sa.ForeignKeyConstraint(['folder_id'], ['folders.id'], ondelete='SET NULL'),
    sa.ForeignKeyConstraint(['owner_id'], ['users.id'], ondelete='CASCADE'),
    sa.ForeignKeyConstraint(['project_id'], ['projects.id'], ondelete='CASCADE'),
    sa.PrimaryKeyConstraint('id')
    )


def downgrade() -> None:
    op.drop_table('documents')
    op.drop_table('folders')
