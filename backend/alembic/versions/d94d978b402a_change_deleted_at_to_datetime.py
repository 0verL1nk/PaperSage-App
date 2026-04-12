"""change_deleted_at_to_datetime

Revision ID: d94d978b402a
Revises: 4885f83a21bf
Create Date: 2026-04-12 19:28:48.912198

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'd94d978b402a'
down_revision: Union[str, Sequence[str], None] = '4885f83a21bf'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Change deleted_at from String(50) to DateTime.

    SQLite does not support ALTER COLUMN directly, so we recreate the table.
    """
    # Step 1: Create new table with correct schema
    op.create_table(
        'projects_new',
        sa.Column('id', sa.NUMERIC(), primary_key=True, nullable=False),
        sa.Column('name', sa.VARCHAR(100), nullable=False),
        sa.Column('description', sa.TEXT(), nullable=True),
        sa.Column('owner_id', sa.NUMERIC(), nullable=False),
        sa.Column('group_id', sa.NUMERIC(), nullable=True),
        sa.Column('status', sa.VARCHAR(20), nullable=False),
        sa.Column('is_deleted', sa.BOOLEAN(), nullable=False),
        sa.Column('deleted_at', sa.DATETIME(), nullable=True),
        sa.Column('created_at', sa.DATETIME(), nullable=False),
        sa.Column('updated_at', sa.DATETIME(), nullable=False),
        sa.ForeignKeyConstraint(['owner_id'], ['users.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['group_id'], ['project_groups.id'], ondelete='SET NULL'),
    )

    # Step 2: Copy data (deleted_at values are stored as ISO strings, will be interpreted as strings)
    op.execute("""
        INSERT INTO projects_new (id, name, description, owner_id, group_id, status, is_deleted, deleted_at, created_at, updated_at)
        SELECT id, name, description, owner_id, group_id, status, is_deleted, deleted_at, created_at, updated_at
        FROM projects
    """)

    # Step 3: Drop old table
    op.drop_table('projects')

    # Step 4: Rename new table
    op.rename_table('projects_new', 'projects')


def downgrade() -> None:
    """Revert deleted_at from DateTime to String(50)."""
    # Recreate table with String deleted_at
    op.create_table(
        'projects_old',
        sa.Column('id', sa.NUMERIC(), primary_key=True, nullable=False),
        sa.Column('name', sa.VARCHAR(100), nullable=False),
        sa.Column('description', sa.TEXT(), nullable=True),
        sa.Column('owner_id', sa.NUMERIC(), nullable=False),
        sa.Column('group_id', sa.NUMERIC(), nullable=True),
        sa.Column('status', sa.VARCHAR(20), nullable=False),
        sa.Column('is_deleted', sa.BOOLEAN(), nullable=False),
        sa.Column('deleted_at', sa.VARCHAR(50), nullable=True),
        sa.Column('created_at', sa.DATETIME(), nullable=False),
        sa.Column('updated_at', sa.DATETIME(), nullable=False),
        sa.ForeignKeyConstraint(['owner_id'], ['users.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['group_id'], ['project_groups.id'], ondelete='SET NULL'),
    )

    op.execute("""
        INSERT INTO projects_old (id, name, description, owner_id, group_id, status, is_deleted, deleted_at, created_at, updated_at)
        SELECT id, name, description, owner_id, group_id, status, is_deleted, deleted_at, created_at, updated_at
        FROM projects
    """)

    op.drop_table('projects')
    op.rename_table('projects_old', 'projects')
