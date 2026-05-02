"""add_sync_state_to_compile_jobs

Revision ID: 6f0e95c16c1e
Revises: 05b22bee6786
Create Date: 2026-04-26 23:27:19.459595

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = '6f0e95c16c1e'
down_revision: Union[str, None] = '05b22bee6786'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    op.add_column(
        'compile_jobs',
        sa.Column(
            'sync_state',
            sa.JSON(),
            nullable=True,
            comment='Snapshot of {file_path: content_hash} from last successful compile',
        ),
    )


def downgrade() -> None:
    op.drop_column('compile_jobs', 'sync_state')
