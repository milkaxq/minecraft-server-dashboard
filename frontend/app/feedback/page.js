import { Suspense } from "react";
import { Feedback } from "../../components/feedback";
export default function Page() {
  return (
    <Suspense fallback={<div className="loading-full">Opening feedback…</div>}>
      <Feedback />
    </Suspense>
  );
}
