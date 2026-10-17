import networkx as nx

# first output marks "imports done"
print("ready: w-networkx")

g = nx.barabasi_albert_graph(20_000, 3, seed=0)
pr = nx.pagerank(g)
top = max(pr, key=pr.get)
print(f"nodes={g.number_of_nodes()} top={top} shortest(0,{top})={nx.shortest_path_length(g, 0, top)}")
