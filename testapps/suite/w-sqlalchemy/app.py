from sqlalchemy import Integer, String, create_engine, func, select
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column

# first output marks "imports done"
print("ready: w-sqlalchemy")

class Base(DeclarativeBase):
    pass

class User(Base):
    __tablename__ = "users"
    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    team: Mapped[str] = mapped_column(String(20))

engine = create_engine("sqlite://")
Base.metadata.create_all(engine)
with Session(engine) as s:
    s.add_all([User(team=f"team{i % 10}") for i in range(20_000)])
    s.commit()
    rows = s.execute(select(User.team, func.count()).group_by(User.team)).all()
print(f"teams={len(rows)}")
