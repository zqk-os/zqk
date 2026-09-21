import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import App from './App';
import '@testing-library/jest-dom';

describe('Marketing Site', () => {
  it('shows what zqk is', () => {
    render(<App />);
    expect(screen.getByText(/what zqk is/i)).toBeInTheDocument();
  });

  it('shows install path', () => {
    render(<App />);
    expect(screen.getAllByText(/curl/i)[0]).toBeInTheDocument();
  });

  it('shows 5-minute value', () => {
    render(<App />);
    expect(screen.getAllByText(/5-minute/i)[0]).toBeInTheDocument();
  });

  it('shows where to get help', () => {
    render(<App />);
    expect(screen.getByText(/help/i)).toBeInTheDocument();
  });

  it('does not show executive partner portal', () => {
    render(<App />);
    expect(screen.queryByText(/Executive Partner Portal/i)).not.toBeInTheDocument();
  });
});
