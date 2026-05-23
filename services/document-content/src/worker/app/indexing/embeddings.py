from collections.abc import Iterable

from openai import OpenAI

from ..config import EMBEDDING_BATCH_SIZE, EMBEDDING_DIMENSIONS, EMBEDDING_MODEL


def batched(items: list[str], batch_size: int) -> Iterable[list[str]]:
    for start in range(0, len(items), batch_size):
        yield items[start : start + batch_size]


def create_embeddings(texts: list[str]) -> list[list[float]]:
    if not texts:
        return []

    client = OpenAI()
    embeddings: list[list[float]] = []

    for batch in batched(texts, EMBEDDING_BATCH_SIZE):
        request = {
            "model": EMBEDDING_MODEL,
            "input": batch,
        }
        if EMBEDDING_DIMENSIONS and EMBEDDING_MODEL.startswith("text-embedding-3"):
            request["dimensions"] = EMBEDDING_DIMENSIONS

        response = client.embeddings.create(**request)
        embeddings.extend(item.embedding for item in response.data)

    return embeddings
