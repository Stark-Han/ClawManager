import { createContext, useContext, useEffect, type ReactNode } from 'react';
import { useAuthStore } from '../stores/authStore';

interface AuthContextType {
  isAuthenticated: boolean;
  isLoading: boolean;
  user: any;
  error: string | null;
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const store = useAuthStore();

  useEffect(() => {
    // Check if user is already logged in
    store.fetchCurrentUser();
  }, []);

  const value: AuthContextType = {
    isAuthenticated: store.isAuthenticated,
    isLoading: store.isLoading,
    user: store.user,
    error: store.error,
    login: store.login,
    register: store.register,
    logout: store.logout,
    clearError: store.clearError,
  };

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
