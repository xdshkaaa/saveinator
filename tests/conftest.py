import pytest
import sqlalchemy as sa

from db.models import Base
from db.session import async_session_factory


@pytest.fixture(autouse=True)
async def _db():
    """Create tables at start of test session, drop at end."""
    engine = async_session_factory.kw["bind"]
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)


@pytest.fixture
async def db_session():
    async with async_session_factory() as session:
        yield session
