// Tarjan's strongly connected components algorithm.
// Returns SCCs in reverse topological order (sinks first), which is convenient
// for downstream rank assignment on the condensation DAG.

interface Edge {
  source: string
  target: string
}

export function findSccs(nodeIds: string[], edges: Edge[]): string[][] {
  const adjacency = new Map<string, string[]>()
  for (const id of nodeIds) adjacency.set(id, [])
  for (const edge of edges) {
    if (adjacency.has(edge.source) && adjacency.has(edge.target)) {
      adjacency.get(edge.source)!.push(edge.target)
    }
  }

  const index = new Map<string, number>()
  const lowlink = new Map<string, number>()
  const onStack = new Set<string>()
  const stack: string[] = []
  const sccs: string[][] = []
  let counter = 0

  // Iterative Tarjan to avoid stack overflow on large graphs.
  const visit = (root: string) => {
    interface Frame {
      node: string
      neighbors: string[]
      next: number
    }
    const frames: Frame[] = []

    const push = (node: string) => {
      index.set(node, counter)
      lowlink.set(node, counter)
      counter++
      stack.push(node)
      onStack.add(node)
      frames.push({ node, neighbors: adjacency.get(node) ?? [], next: 0 })
    }

    push(root)

    while (frames.length > 0) {
      const frame = frames[frames.length - 1]!
      if (frame.next < frame.neighbors.length) {
        const next = frame.neighbors[frame.next]!
        frame.next += 1
        if (!index.has(next)) {
          push(next)
        } else if (onStack.has(next)) {
          lowlink.set(
            frame.node,
            Math.min(lowlink.get(frame.node)!, index.get(next)!),
          )
        }
        continue
      }

      // All neighbors processed: maybe emit SCC, then pop.
      if (lowlink.get(frame.node) === index.get(frame.node)) {
        const scc: string[] = []
        while (true) {
          const w = stack.pop()!
          onStack.delete(w)
          scc.push(w)
          if (w === frame.node) break
        }
        sccs.push(scc)
      }
      frames.pop()
      if (frames.length > 0) {
        const parent = frames[frames.length - 1]!
        lowlink.set(
          parent.node,
          Math.min(lowlink.get(parent.node)!, lowlink.get(frame.node)!),
        )
      }
    }
  }

  for (const id of nodeIds) {
    if (!index.has(id)) visit(id)
  }

  return sccs
}
