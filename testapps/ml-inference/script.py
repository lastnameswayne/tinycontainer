"""
Text similarity search with TF-IDF — the kind of workload Modal runs.
Uses scikit-learn for vectorization and cosine similarity.
"""

import time
import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity

# Corpus: mix of ML papers, product reviews, code docs
DOCUMENTS = [
    "Attention is all you need. We propose a new network architecture based solely on attention mechanisms.",
    "BERT pre-trains deep bidirectional representations from unlabeled text by jointly conditioning on both left and right context.",
    "GPT-4 is a large multimodal model that exhibits human-level performance on various professional benchmarks.",
    "Docker containers provide lightweight isolation for running applications in reproducible environments.",
    "Kubernetes orchestrates containerized workloads across a cluster of machines with automatic scaling.",
    "Modal lets you run any Python function in the cloud with a single decorator. No infrastructure to manage.",
    "FastAPI is a modern web framework for building APIs with Python based on standard type hints.",
    "NumPy provides support for large multi-dimensional arrays and matrices along with mathematical functions.",
    "The transformer architecture uses self-attention to process sequences in parallel rather than sequentially.",
    "Serverless computing abstracts away infrastructure management so developers focus purely on code.",
    "Batch inference pipelines process large datasets by distributing work across many GPU workers in parallel.",
    "Vector databases store embeddings and enable fast approximate nearest neighbor search at scale.",
    "Fine-tuning a pre-trained language model on domain-specific data dramatically improves task performance.",
    "Cloud container runtimes execute user code in isolated sandboxes with sub-second cold start times.",
    "The Python GIL limits true parallelism but async IO and multiprocessing provide effective workarounds.",
]

QUERIES = [
    "running code in the cloud without managing servers",
    "machine learning language models",
    "container orchestration and deployment",
]

def main():
    print("=" * 64)
    print("  Semantic Search Engine — TF-IDF + Cosine Similarity")
    print("  Running on sway cloud container runtime")
    print("=" * 64)
    print()

    start = time.time()

    # Build TF-IDF index
    vectorizer = TfidfVectorizer(stop_words="english")
    doc_vectors = vectorizer.fit_transform(DOCUMENTS)
    vocab_size = len(vectorizer.vocabulary_)

    index_time = time.time() - start
    print(f"  Indexed {len(DOCUMENTS)} documents ({vocab_size} terms) in {index_time*1000:.1f}ms")
    print()

    # Run queries
    for query in QUERIES:
        q_vec = vectorizer.transform([query])
        scores = cosine_similarity(q_vec, doc_vectors).flatten()
        ranked = np.argsort(scores)[::-1]

        print(f'  Query: "{query}"')
        for rank, idx in enumerate(ranked[:3], 1):
            score = scores[idx]
            snippet = DOCUMENTS[idx][:70]
            print(f"    {rank}. [{score:.3f}] {snippet}...")
        print()

    elapsed = time.time() - start
    qps = len(QUERIES) / elapsed

    print(f"  Total: {elapsed*1000:.1f}ms | {qps:,.0f} queries/sec")
    print("=" * 64)

if __name__ == "__main__":
    main()
