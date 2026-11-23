import type { NextConfig } from 'next';
const nextConfig: NextConfig = {
  outputFileTracingRoot: __dirname,
  distDir: process.env.NODE_ENV === 'development' ? '.next-dev' : '.next',
  async rewrites() { return [{ source: '/api/:path*', destination: `${process.env.API_INTERNAL_URL || 'http://127.0.0.1:8080'}/:path*` }]; },
};
export default nextConfig;
