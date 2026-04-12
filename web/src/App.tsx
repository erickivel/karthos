import { Routes, Route } from 'react-router-dom'
import { Shell } from '@/components/Shell/Shell'
import { ClusterOverview } from '@/views/ClusterOverview/ClusterOverview'
import { TopologyGraph } from '@/views/TopologyGraph/TopologyGraph'
import { MetricsExplorer } from '@/views/MetricsExplorer/MetricsExplorer'

export default function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<ClusterOverview />} />
        <Route path="topology" element={<TopologyGraph />} />
        <Route path="metrics" element={<MetricsExplorer />} />
      </Route>
    </Routes>
  )
}
