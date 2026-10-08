import { Checkout } from "../../../components/billing";
export default async function Page({ params }) {
  const { id } = await params;
  return <Checkout id={id} />;
}
