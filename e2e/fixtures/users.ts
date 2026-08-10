export const users = {
  admin: {
    username: "admin",
    password: "admin123",
    role: "admin",
    storageState: ".auth/admin.json"
  },
  user: {
    username: "e2euser",
    email: "e2euser@clawmanager.local",
    password: "user12345",
    role: "user"
  }
} as const;
