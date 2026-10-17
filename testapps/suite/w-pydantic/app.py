from datetime import datetime
from pydantic import BaseModel, Field

# first output marks "imports done"
print("ready: w-pydantic")

class Event(BaseModel):
    id: int
    at: datetime
    tags: list[str] = Field(default_factory=list)
    score: float

raw = [{"id": i, "at": "2024-05-01T12:00:00", "tags": ["a", "b"], "score": str(i / 3)} for i in range(50_000)]
events = [Event.model_validate(r) for r in raw]
print(f"events={len(events)} last_score={events[-1].score:.2f}")
