import { create } from 'zustand'

type ConnectionStatus = 'connected' | 'degraded' | 'disconnected'

interface AppState {
  selectedEntityId: string | null
  connectionStatus: ConnectionStatus
  setSelectedEntityId: (id: string | null) => void
  setConnectionStatus: (status: ConnectionStatus) => void
}

export const useAppStore = create<AppState>((set) => ({
  selectedEntityId: null,
  connectionStatus: 'disconnected',
  setSelectedEntityId: (id) => set({ selectedEntityId: id }),
  setConnectionStatus: (status) => set({ connectionStatus: status }),
}))
