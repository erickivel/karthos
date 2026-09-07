import { create } from 'zustand'

type ConnectionStatus = 'connected' | 'degraded' | 'disconnected'

interface AppState {
  selectedEntityId: string | null
  relationshipCardEntityId: string | null
  relationshipCardPinned: boolean
  connectionStatus: ConnectionStatus
  setSelectedEntityId: (id: string | null) => void
  openRelationshipCard: (id: string) => void
  closeRelationshipCard: () => void
  setRelationshipCardPinned: (pinned: boolean) => void
  setConnectionStatus: (status: ConnectionStatus) => void
}

export const useAppStore = create<AppState>((set) => ({
  selectedEntityId: null,
  relationshipCardEntityId: null,
  relationshipCardPinned: false,
  connectionStatus: 'disconnected',
  setSelectedEntityId: (id) => set({ selectedEntityId: id }),
  openRelationshipCard: (id) =>
    set({ relationshipCardEntityId: id, relationshipCardPinned: false }),
  closeRelationshipCard: () =>
    set({ relationshipCardEntityId: null, relationshipCardPinned: false }),
  setRelationshipCardPinned: (pinned) => set({ relationshipCardPinned: pinned }),
  setConnectionStatus: (status) => set({ connectionStatus: status }),
}))
