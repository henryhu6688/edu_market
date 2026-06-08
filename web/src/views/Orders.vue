<template>
  <div class="orders-page">
    <h2>我的订单</h2>
    <div class="status-filter">
      <button :class="{ active: status === '' }" @click="status = ''; fetchOrders()">全部</button>
      <button :class="{ active: status === 'pending' }" @click="status = 'pending'; fetchOrders()">待支付</button>
      <button :class="{ active: status === 'paid' }" @click="status = 'paid'; fetchOrders()">已支付</button>
      <button :class="{ active: status === 'cancelled' }" @click="status = 'cancelled'; fetchOrders()">已取消</button>
    </div>

    <div v-if="orders.length" class="order-list">
      <div v-for="order in orders" :key="order.id" class="order-item">
        <div class="order-info">
          <span class="order-no">订单号: {{ order.order_no }}</span>
          <span class="order-course">{{ order.course?.title || '未知课程' }}</span>
          <span class="order-amount">¥{{ order.amount }}</span>
        </div>
        <div class="order-right">
          <span class="status-tag" :class="order.status">{{ statusMap[order.status] }}</span>
          <button v-if="order.status === 'pending'" class="btn-pay" @click="handlePay(order.order_no)">去支付</button>
        </div>
      </div>
    </div>
    <div v-else class="empty">暂无订单</div>

    <Pagination :page="page" :pageSize="pageSize" :total="total" @change="p => { page = p; fetchOrders() }" />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getOrders, payOrder } from '@/api/order'
import Pagination from '@/components/Pagination.vue'

const statusMap = { pending: '待支付', paid: '已支付', cancelled: '已取消' }
const orders = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const status = ref('')

async function fetchOrders() {
  try {
    const res = await getOrders({ page: page.value, page_size: pageSize.value, status: status.value })
    if (res.code === 200) {
      orders.value = res.data.list || []
      total.value = res.data.total || 0
    }
  } catch (e) { /* 静默 */ }
}

async function handlePay(orderNo) {
  try {
    await payOrder(orderNo)
    alert('支付成功')
    fetchOrders()
  } catch (e) { alert(e?.message || '支付失败') }
}

onMounted(fetchOrders)
</script>

<style scoped>
.orders-page { max-width: 800px; margin: 0 auto; }
.orders-page h2 { margin-bottom: 20px; }
.status-filter { display: flex; gap: 8px; margin-bottom: 20px; }
.status-filter button {
  padding: 6px 16px; border: 1px solid #ddd; background: #fff;
  border-radius: 20px; cursor: pointer; font-size: 13px;
}
.status-filter button.active { background: #4f46e5; color: #fff; border-color: #4f46e5; }
.order-item {
  background: #fff; padding: 16px 20px; border-radius: 10px;
  margin-bottom: 10px; display: flex; justify-content: space-between;
  align-items: center; box-shadow: 0 1px 4px rgba(0,0,0,0.04);
}
.order-info { display: flex; flex-direction: column; gap: 4px; }
.order-no { font-size: 12px; color: #999; }
.order-course { font-weight: 600; }
.order-amount { font-size: 18px; font-weight: 700; color: #e74c3c; }
.order-right { display: flex; align-items: center; gap: 12px; }
.status-tag { font-size: 12px; padding: 4px 10px; border-radius: 4px; }
.status-tag.pending { background: #fef3c7; color: #d97706; }
.status-tag.paid { background: #d1fae5; color: #059669; }
.status-tag.cancelled { background: #f1f5f9; color: #94a3b8; }
.btn-pay {
  padding: 6px 16px; background: #4f46e5; color: #fff;
  border: none; border-radius: 6px; cursor: pointer; font-size: 13px;
}
.btn-pay:hover { background: #4338ca; }
.empty { text-align: center; padding: 60px 0; color: #999; }
</style>
