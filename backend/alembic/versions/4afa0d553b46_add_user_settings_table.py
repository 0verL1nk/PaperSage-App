"""add user settings table

Revision ID: 4afa0d553b46
Revises: 6f0e95c16c1e
Create Date: 2026-05-02 21:03:25.530520

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision: str = '4afa0d553b46'
down_revision: Union[str, None] = '6f0e95c16c1e'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.create_table(
        'user_settings',
        sa.Column('user_id', sa.String(length=32), nullable=False),
        sa.Column('category', sa.String(length=100), nullable=False),
        sa.Column('settings', sa.JSON(), server_default='{}', nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('user_id', 'category'),
    )


def downgrade() -> None:
    op.drop_table('user_settings')
