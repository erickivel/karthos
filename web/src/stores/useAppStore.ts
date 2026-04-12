import { create } from 'zustand'

type ConnectionStatus = 'connected' | 'degraded' | 'disconnected'

interface AppState {
  selectedEntityId: string | null
  connectionStatus: ConnectionStatus
  sidebarCollapsed: boolean
  setSelectedEntityId: (id: string | null) => void
  setConnectionStatus: (status: ConnectionStatus) => void
  toggleSidebar: () => void
}

export const useAppStore = create<AppState>((set) => ({
  selectedEntityId: null,
  connectionStatus: 'disconnected',
  sidebarCollapsed: false,
  setSelectedEntityId: (id) => set({ selectedEntityId: id }),
  setConnectionStatus: (status) => set({ connectionStatus: status }),
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
}))
