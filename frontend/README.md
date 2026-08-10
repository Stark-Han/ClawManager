# ClawReef Frontend

Virtual Desktop Management Platform - React Frontend

## Tech Stack

- React 19
- TypeScript 5.9+
- Vite 7+
- Tailwind CSS 4
- React Router 6
- Zustand (State Management)
- Axios (HTTP Client)
- TanStack Query (Data Fetching)

## Quick Start

### Install Dependencies

```bash
npm install
```

### Development

```bash
npm run dev
```

### Build

```bash
npm run build
```

### Environment Variables

Create `.env.development` file:

```
VITE_API_URL=http://localhost:9001/api/v1
```

Frontend dev server runs on port **9002**.

## Project Structure

```
src/
├── components/       # UI Components
│   ├── ui/          # Base UI components
│   ├── layout/      # Layout components
│   ├── common/      # Shared components
│   └── forms/       # Form components
├── pages/           # Route pages
│   ├── auth/        # Authentication pages
│   ├── dashboard/   # Dashboard pages
│   ├── instances/   # Instance management
│   └── admin/       # Admin pages
├── services/        # API services
├── stores/          # State management
├── types/           # TypeScript types
├── contexts/        # React contexts
├── router/          # Route configuration
└── lib/             # Utilities
```

## Available Scripts

- `npm run dev` - Start development server
- `npm run build` - Build for production
- `npm run preview` - Preview production build
- `npm run lint` - Run ESLint
