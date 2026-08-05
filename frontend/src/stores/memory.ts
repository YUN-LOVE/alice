import { defineStore } from 'pinia'
import { getMemoryBlock, searchMemory, type BlockEntry, type SearchResult } from '../services/api'

export const useMemoryStore = defineStore('memory', {
  state: () => ({
    entries: [] as BlockEntry[],
    total: 0,
    loading: false,
    searching: false,
    searchResults: [] as SearchResult[],
    searchQuery: '',
    error: '',
  }),

  actions: {
    async refresh() {
      this.loading = true
      this.error = ''
      try {
        const data = await getMemoryBlock()
        this.entries = data.entries
        this.total = data.total
      } catch (e: any) {
        this.error = e.message ?? '获取记忆失败'
      } finally {
        this.loading = false
      }
    },

    async search(query: string) {
      this.searchQuery = query
      if (!query.trim()) {
        this.searchResults = []
        return
      }
      this.searching = true
      try {
        const data = await searchMemory(query)
        this.searchResults = data.results
      } catch (e: any) {
        this.error = e.message ?? '搜索失败'
      } finally {
        this.searching = false
      }
    },

    reset() {
      this.entries = []
      this.total = 0
      this.searchResults = []
      this.searchQuery = ''
    },
  },
})
