export default {
  server: {
    proxy: {
      '^/files.*': 'http://127.0.0.1:8080'
    },
  },
}
