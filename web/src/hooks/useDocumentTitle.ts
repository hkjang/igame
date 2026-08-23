import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { documentTitleForPath } from '../pages/routeTitles';

/**
 * Keeps the browser tab in step with the route. A single-page app never
 * reloads, so without this every page, bookmark and history entry is labelled
 * with whatever the document started as.
 */
export function useDocumentTitle(serviceName: string) {
  const { pathname } = useLocation();
  useEffect(() => {
    document.title = documentTitleForPath(pathname, serviceName);
  }, [pathname, serviceName]);
}
